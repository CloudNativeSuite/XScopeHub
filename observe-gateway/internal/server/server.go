package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/xscopehub/observe-gateway/internal/audit"
	"github.com/xscopehub/observe-gateway/internal/auth"
	"github.com/xscopehub/observe-gateway/internal/backend"
	"github.com/xscopehub/observe-gateway/internal/cache"
	"github.com/xscopehub/observe-gateway/internal/config"
	"github.com/xscopehub/observe-gateway/internal/limiter"
	"github.com/xscopehub/observe-gateway/internal/query"
)

// Server represents the HTTP API server.
type Server struct {
	cfg      config.Config
	router   chi.Router
	auth     *auth.Authenticator
	backend  *backend.Client
	cache    *cache.Cache
	limiter  *limiter.Limiter
	auditLog *audit.Logger

	activeRequests int64
}

// New constructs a server with all dependencies wired.
func New(cfg config.Config, auth *auth.Authenticator, backend *backend.Client, cache *cache.Cache, limiter *limiter.Limiter, auditLog *audit.Logger) *Server {
	s := &Server{
		cfg:      cfg,
		auth:     auth,
		backend:  backend,
		cache:    cache,
		limiter:  limiter,
		auditLog: auditLog,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(2 * time.Minute))

	r.Post("/api/query", s.handleQuery)

	s.router = r
	return s
}

// Handler exposes the HTTP handler for embedding.
func (s *Server) Handler() http.Handler {
	return s.router
}

// Run starts the HTTP server until context cancellation.
func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:         s.cfg.Server.Address,
		Handler:      s.Handler(),
		ReadTimeout:  s.cfg.Server.ReadTimeout,
		WriteTimeout: s.cfg.Server.WriteTimeout,
		IdleTimeout:  s.cfg.Server.IdleTimeout,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		err := <-errCh
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	atomic.AddInt64(&s.activeRequests, 1)
	defer atomic.AddInt64(&s.activeRequests, -1)

	start := time.Now()

	var req query.Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request: %v", err))
		s.auditLog.Log(audit.Entry{Tenant: "", User: "", Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: err.Error()})
		return
	}

	req.Lang = strings.ToLower(req.Lang)
	if req.Query == "" {
		s.writeError(w, http.StatusBadRequest, "query is required")
		s.auditLog.Log(audit.Entry{Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: "query is required"})
		return
	}

	if req.Step != "" {
		if _, err := req.StepDuration(); err != nil {
			s.writeError(w, http.StatusBadRequest, "invalid step duration")
			s.auditLog.Log(audit.Entry{Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: "invalid step"})
			return
		}
	}

	tenant := r.Header.Get("X-Tenant")
	user := r.Header.Get("X-User")
	if s.auth != nil {
		if t, u, err := s.auth.Verify(r); err == nil {
			tenant, user = t, u
		} else {
			s.writeError(w, http.StatusUnauthorized, err.Error())
			s.auditLog.Log(audit.Entry{Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: err.Error()})
			return
		}
	}
	if tenant == "" {
		s.writeError(w, http.StatusBadRequest, "tenant is required")
		s.auditLog.Log(audit.Entry{Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: "tenant missing"})
		return
	}

	if err := s.validate(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, err.Error())
		s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: err.Error()})
		return
	}

	if s.limiter != nil {
		if err := s.limiter.Allow(r.Context(), tenant); err != nil {
			status := http.StatusTooManyRequests
			if !errors.Is(err, limiter.ErrRateLimited) {
				status = http.StatusInternalServerError
			}
			s.writeError(w, status, err.Error())
			s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: err.Error()})
			return
		}
	}

	cacheKey := buildCacheKey(req, tenant)
	if data, ok := s.cache.Get(r.Context(), cacheKey); ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(data)

		var cachedResp query.Response
		if err := json.Unmarshal(data, &cachedResp); err == nil {
			s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Cached: true, Backend: cachedResp.Stats.Backend, Cost: cachedResp.Stats.Cost})
		} else {
			s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Cached: true, Backend: "cache"})
		}
		return
	}

	result, err := s.dispatch(r.Context(), tenant, req)
	if err != nil {
		status := http.StatusBadGateway
		var unsupported *backend.UnsupportedError
		if errors.As(err, &unsupported) {
			status = http.StatusBadRequest
		}
		s.writeError(w, status, err.Error())
		s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: err.Error()})
		return
	}

	resp := query.Response{
		Lang:   req.Lang,
		Tenant: tenant,
		Result: result.Payload,
		Stats: query.Stats{
			Backend:    result.Backend,
			Cached:     false,
			DurationMS: time.Since(start).Milliseconds(),
			Cost:       result.Cost,
		},
	}

	payload, err := json.Marshal(resp)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "marshal response failed")
		s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Error: err.Error(), Backend: result.Backend})
		return
	}

	s.cache.Set(r.Context(), cacheKey, payload, int64(len(payload)))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(payload)

	s.auditLog.Log(audit.Entry{Tenant: tenant, User: user, Lang: req.Lang, Query: req.Query, Duration: time.Since(start), Cost: result.Cost, Backend: result.Backend})
}

func (s *Server) dispatch(ctx context.Context, tenant string, req query.Request) (backend.Result, error) {
	switch req.Lang {
	case "promql":
		return s.backend.QueryPromQL(ctx, tenant, req)
	case "logql":
		return s.backend.QueryLogQL(ctx, tenant, req)
	case "traceql":
		return s.backend.QueryTraceQL(ctx, tenant, req)
	default:
		return backend.Result{}, fmt.Errorf("unsupported language: %s", req.Lang)
	}
}

func (s *Server) validate(req *query.Request) error {
	switch req.Lang {
	case "promql":
		return nil
	case "logql", "traceql":
		if !req.HasTimeRange() {
			return fmt.Errorf("%s requires start and end", req.Lang)
		}
		if req.Start.After(req.End) {
			return fmt.Errorf("start must be before end")
		}
		return nil
	default:
		return fmt.Errorf("unsupported language: %s", req.Lang)
	}
}

func (s *Server) writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	payload, _ := json.Marshal(map[string]string{"error": msg})
	w.Write(payload)
}

func buildCacheKey(req query.Request, tenant string) string {
	var parts []string
	parts = append(parts, strings.ToLower(req.Lang), req.Query, tenant)
	if !req.Start.IsZero() {
		parts = append(parts, req.Start.UTC().Format(time.RFC3339Nano))
	}
	if !req.End.IsZero() {
		parts = append(parts, req.End.UTC().Format(time.RFC3339Nano))
	}
	if req.Step != "" {
		parts = append(parts, req.Step)
	}
	if req.Normalize {
		parts = append(parts, "normalize=true")
	}
	return strings.Join(parts, "|")
}
