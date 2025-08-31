package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// RegisterRoutes wires all HTTP handlers for the agent modules.
func RegisterRoutes(r gin.IRoutes) {
	r.GET("/ingest/*any", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "sensor", "status": "ok"})
	})
	r.GET("/analyze/run", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "analyst", "status": "ok"})
	})
	r.GET("/plan/generate", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "planner", "status": "ok"})
	})
	r.GET("/gate/eval", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "gatekeeper", "status": "ok"})
	})
	r.GET("/adapter/exec", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "executor", "status": "ok"})
	})
	r.GET("/kb/ingest", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "librarian", "status": "ok"})
	})
	r.GET("/case/*any", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"module": "orchestrator", "status": "ok"})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
}
