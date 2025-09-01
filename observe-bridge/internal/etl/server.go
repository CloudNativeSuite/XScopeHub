package etl

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/xscopehub/xscopehub/etl/jobs"
	"github.com/xscopehub/xscopehub/etl/pkg/ansible"
	"github.com/xscopehub/xscopehub/etl/pkg/events"
	"github.com/xscopehub/xscopehub/etl/pkg/iac"
	"github.com/xscopehub/xscopehub/etl/pkg/oo"
	"github.com/xscopehub/xscopehub/etl/pkg/pgw"
	"github.com/xscopehub/xscopehub/etl/pkg/scheduler"
	"github.com/xscopehub/xscopehub/etl/pkg/window"
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
	r.Use(gin.Logger())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.StaticFile("/openapi.yaml", "etl/api/openapi.yaml")

	// Dataflow entry
	r.GET("/oo/stream", handleOOStream)
	r.POST("/pgw/flush", handlePGWFlush)
	r.POST("/pgw/topo/edges", handlePGWTopoEdges)

	// Jobs
	r.POST("/jobs/ooagg/run", handleJobOOAgg)
	r.POST("/jobs/age_refresh/run", handleJobAGERefresh)
	r.POST("/jobs/topo/iac/run", handleJobTopoIAC)
	r.POST("/jobs/topo/ansible/run", handleJobTopoAnsible)

	// Events and scheduler
	r.POST("/events/enqueue", handleEventsEnqueue)
	r.POST("/scheduler/tick", handleSchedulerTick)

	// Topology discovery
	r.GET("/topo/iac/discover", handleIACDiscover)
	r.GET("/topo/ansible/extract", handleAnsibleExtract)

	return &Server{engine: r, cfg: cfg}
}

func parseWindowParams(c *gin.Context) (window.Window, error) {
	fromStr := c.Query("from")
	toStr := c.Query("to")
	if fromStr == "" || toStr == "" {
		return window.Window{}, fmt.Errorf("missing from/to")
	}
	from, err := time.Parse(time.RFC3339, fromStr)
	if err != nil {
		return window.Window{}, err
	}
	to, err := time.Parse(time.RFC3339, toStr)
	if err != nil {
		return window.Window{}, err
	}
	return window.Window{From: from, To: to}, nil
}

func handleOOStream(c *gin.Context) {
	tenant := c.Query("tenant")
	w, err := parseWindowParams(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := oo.Stream(c.Request.Context(), tenant, w, func(rec oo.Record) {}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handlePGWFlush(c *gin.Context) {
	tenant := c.Query("tenant")
	w := window.Window{}
	body, _ := io.ReadAll(c.Request.Body)
	if err := pgw.Flush(c.Request.Context(), tenant, w, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handlePGWTopoEdges(c *gin.Context) {
	tenant := c.Query("tenant")
	if err := pgw.UpsertTopoEdges(c.Request.Context(), tenant, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleJobOOAgg(c *gin.Context) {
	tenant := c.Query("tenant")
	w := window.Window{}
	if err := jobs.RunOOAgg(c.Request.Context(), tenant, w); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleJobAGERefresh(c *gin.Context) {
	tenant := c.Query("tenant")
	w := window.Window{}
	if err := jobs.RunAGERefresh(c.Request.Context(), tenant, w); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleJobTopoIAC(c *gin.Context) {
	tenant := c.Query("tenant")
	w := window.Window{}
	if err := jobs.RunTopoIAC(c.Request.Context(), tenant, w); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleJobTopoAnsible(c *gin.Context) {
	tenant := c.Query("tenant")
	w := window.Window{}
	if err := jobs.RunTopoAnsible(c.Request.Context(), tenant, w); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleEventsEnqueue(c *gin.Context) {
	body, _ := io.ReadAll(c.Request.Body)
	if err := events.Enqueue(c.Request.Context(), body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleSchedulerTick(c *gin.Context) {
	if err := scheduler.Tick(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusOK)
}

func handleIACDiscover(c *gin.Context) {
	tenant := c.Query("tenant")
	edges, err := iac.Discover(c.Request.Context(), tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, edges)
}

func handleAnsibleExtract(c *gin.Context) {
	tenant := c.Query("tenant")
	edges, err := ansible.ExtractDeps(c.Request.Context(), tenant)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, edges)
}

// Run starts the HTTP server using the configured listen address.
func (s *Server) Run() error {
	if s.cfg == nil || s.cfg.Server.API.Listen == "" {
		return fmt.Errorf("server listen address not configured")
	}
	return s.engine.Run(s.cfg.Server.API.Listen)
}
