package etl

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/xscopehub/xscopehub/internal/etl/config"
)

// Server wraps the HTTP engine and configuration.
type Server struct {
	engine *gin.Engine
	cfg    *config.Config
}

// NewServer creates a server with basic health and metrics endpoints.
func NewServer(cfg *config.Config) *Server {
	r := gin.New()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.StaticFile("/openapi.yaml", "etl/api/openapi.yaml")
	return &Server{engine: r, cfg: cfg}
}

// Run starts the HTTP server using the configured listen address.
func (s *Server) Run() error {
	if s.cfg == nil || s.cfg.Server.API.Listen == "" {
		return fmt.Errorf("server listen address not configured")
	}
	return s.engine.Run(s.cfg.Server.API.Listen)
}
