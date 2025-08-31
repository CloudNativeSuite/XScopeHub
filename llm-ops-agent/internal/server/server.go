package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/nats-io/nats.go"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/yourname/XOpsAgent/internal/config"
	dbpkg "github.com/yourname/XOpsAgent/internal/db"
	logpkg "github.com/yourname/XOpsAgent/pkg/log"
)

// Server wraps application dependencies and HTTP router.
type Server struct {
	cfg    config.Config
	router *gin.Engine
	db     *sql.DB
	nats   *nats.Conn
	kafka  *kafka.Writer
	logger *slog.Logger
}

// New creates a new server instance.
func New(cfg config.Config) (*Server, error) {
	logger := logpkg.New("aiops")
	sqlDB, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := sqlDB.Ping(); err != nil {
		return nil, err
	}

	// Initialize sqlc queries to ensure code generation is wired.
	_ = dbpkg.New(sqlDB)

	nc, err := nats.Connect(cfg.NatsURL)
	if err != nil {
		return nil, err
	}

	kw := kafka.NewWriter(kafka.WriterConfig{Brokers: cfg.KafkaBrokers, Topic: "events"})

	r := gin.New()
	r.Use(gin.Logger(), otelgin.Middleware("aiops"))
	r.StaticFile("/openapi.yaml", "/api/openapi.yaml")

	srv := &Server{cfg: cfg, router: r, db: sqlDB, nats: nc, kafka: kw, logger: logger}
	srv.routes()
	return srv, nil
}

func (s *Server) routes() {
	s.router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))
}

// Run starts the HTTP server.
func (s *Server) Run(ctx context.Context) error {
	defer s.nats.Drain()
	defer s.kafka.Close()
	return s.router.Run(fmt.Sprintf(":%d", s.cfg.Port))
}
