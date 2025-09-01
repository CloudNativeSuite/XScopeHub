package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	db "github.com/yourname/XOpsAgent/db/sqlc"
	"github.com/yourname/XOpsAgent/internal/repository"
	"github.com/yourname/XOpsAgent/workflow"
)

// RegisterRoutes wires all HTTP handlers for the agent modules.
type caseService interface {
	CreateCase(ctx context.Context, args repository.CreateCaseArgs) (db.CreateCaseRow, error)
	Transition(ctx context.Context, args repository.TransitionArgs) (db.UpdateCaseStatusRow, error)
}

func RegisterRoutes(r gin.IRoutes, svc caseService) {
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	h := &caseHandler{svc: svc}
	r.POST("/case/create", h.createCase)
	r.PATCH("/case/:id/transition", h.transitionCase)
}

type caseHandler struct {
	svc caseService
}

type createCaseReq struct {
	TenantID int64  `json:"tenant_id"`
	Title    string `json:"title"`
}

func (h *caseHandler) createCase(c *gin.Context) {
	var req createCaseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	idem := c.GetHeader("Idempotency-Key")
	actor := c.GetHeader("X-Actor")
	row, err := h.svc.CreateCase(c.Request.Context(), repository.CreateCaseArgs{
		TenantID: req.TenantID,
		Title:    req.Title,
		Actor:    actor,
		IdemKey:  idem,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"case_id": row.CaseID.String(), "status": row.Status, "version": row.Version})
}

type transitionReq struct {
	Event string `json:"event"`
}

func (h *caseHandler) transitionCase(c *gin.Context) {
	var req transitionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	idStr := c.Param("id")
	uid, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var ver int64
	ifMatch := c.GetHeader("If-Match")
	if ifMatch != "" {
		ver, _ = strconv.ParseInt(ifMatch, 10, 64)
	}
	idem := c.GetHeader("Idempotency-Key")
	actor := c.GetHeader("X-Actor")
	ctx := workflow.Context{Now: time.Now(), Actor: actor}
	row, err := h.svc.Transition(c.Request.Context(), repository.TransitionArgs{
		CaseID:  pgtype.UUID{Bytes: uid, Valid: true},
		Event:   workflow.Event(req.Event),
		Ctx:     ctx,
		IfMatch: ver,
		IdemKey: idem,
		Request: []byte{},
	})
	if err != nil {
		if errors.Is(err, workflow.ErrIllegal) || errors.Is(err, workflow.ErrGuard) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, pgx.ErrNoRows) {
			c.JSON(http.StatusPreconditionFailed, gin.H{"error": "version mismatch"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Header("ETag", fmt.Sprintf("%d", row.Version))
	c.JSON(http.StatusOK, gin.H{"status": row.Status, "version": row.Version})
}
