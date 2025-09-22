package auth

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"

	"github.com/xscopehub/observe-gateway/internal/config"
)

// Context keys for downstream access.
type contextKey string

const (
	// ContextTenantKey holds tenant identifier in request context.
	ContextTenantKey contextKey = "tenant"
	// ContextUserKey holds user identifier in request context.
	ContextUserKey contextKey = "user"
)

// Authenticator performs JWT authentication backed by a JWK set.
type Authenticator struct {
	enabled bool
	cfg     config.AuthConfig

	mu        sync.RWMutex
	set       jwk.Set
	fetchedAt time.Time
	client    *http.Client
}

// New creates an authenticator using the provided configuration.
func New(cfg config.AuthConfig) (*Authenticator, error) {
	a := &Authenticator{enabled: cfg.Enabled, cfg: cfg}
	if !cfg.Enabled {
		return a, nil
	}
	if cfg.JWKSURL == "" {
		return nil, fmt.Errorf("jwks_url required when auth enabled")
	}

	a.client = &http.Client{Timeout: 10 * time.Second}
	if cfg.InsecureTLs {
		a.client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} // #nosec G402
	}

	if err := a.refresh(context.Background()); err != nil {
		return nil, err
	}

	return a, nil
}

// Enabled returns whether authentication is active.
func (a *Authenticator) Enabled() bool {
	if a == nil {
		return false
	}
	return a.enabled
}

// Verify extracts tenant and user information from the request.
func (a *Authenticator) Verify(r *http.Request) (tenant, user string, err error) {
	if a == nil || !a.enabled {
		return r.Header.Get("X-Tenant"), r.Header.Get("X-User"), nil
	}

	header := r.Header.Get("Authorization")
	if header == "" {
		return "", "", errors.New("authorization header required")
	}
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return "", "", errors.New("authorization header must be bearer token")
	}

	tokenString := strings.TrimSpace(header[7:])
	if tokenString == "" {
		return "", "", errors.New("empty bearer token")
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	set, err := a.getKeySet(ctx)
	if err != nil {
		return "", "", err
	}

	options := []jwt.ParseOption{jwt.WithKeySet(set), jwt.WithValidate(true)}
	if len(a.cfg.Audience) > 0 {
		for _, aud := range a.cfg.Audience {
			if aud == "" {
				continue
			}
			options = append(options, jwt.WithAudience(aud))
		}
	}
	if a.cfg.Issuer != "" {
		options = append(options, jwt.WithIssuer(a.cfg.Issuer))
	}

	token, err := jwt.ParseString(tokenString, options...)
	if err != nil {
		return "", "", err
	}

	tenant = claimAsString(token, a.cfg.TenantClaim, "tenant")
	user = claimAsString(token, a.cfg.UserClaim, "sub")

	if tenant == "" {
		tenant = r.Header.Get("X-Tenant")
	}
	if user == "" {
		user = r.Header.Get("X-User")
	}

	return tenant, user, nil
}

func (a *Authenticator) getKeySet(ctx context.Context) (jwk.Set, error) {
	ttl := a.cfg.CacheTTL
	if ttl <= 0 {
		ttl = time.Hour
	}

	a.mu.RLock()
	set := a.set
	fetched := a.fetchedAt
	a.mu.RUnlock()

	if set != nil && time.Since(fetched) < ttl {
		return set, nil
	}

	if err := a.refresh(ctx); err != nil {
		return nil, err
	}

	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.set == nil {
		return nil, errors.New("jwks not loaded")
	}
	return a.set, nil
}

func (a *Authenticator) refresh(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	opts := []jwk.FetchOption{jwk.WithHTTPClient(a.client)}
	set, err := jwk.Fetch(ctx, a.cfg.JWKSURL, opts...)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.set = set
	a.fetchedAt = time.Now()
	return nil
}

func claimAsString(token jwt.Token, claim string, fallback string) string {
	if claim == "" {
		claim = fallback
	}
	if claim == "" {
		return ""
	}
	if value, ok := token.Get(claim); ok {
		switch v := value.(type) {
		case string:
			return v
		case fmt.Stringer:
			return v.String()
		case []string:
			if len(v) > 0 {
				return v[0]
			}
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return ""
}
