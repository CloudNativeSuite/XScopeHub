package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/yourname/XOpsAgent/db/sqlc"
	"github.com/yourname/XOpsAgent/internal/ports"
	"github.com/yourname/XOpsAgent/workflow"
)

type caseState struct {
	status  string
	version int64
}

type fakeService struct {
	cases map[string]*caseState
	idem  map[string]any
}

func newFakeService() *fakeService {
	return &fakeService{cases: map[string]*caseState{}, idem: map[string]any{}}
}

func (f *fakeService) CreateCase(ctx context.Context, args ports.CreateCaseArgs) (db.CreateCaseRow, error) {
	if args.IdemKey != "" {
		if v, ok := f.idem[args.IdemKey]; ok {
			return v.(db.CreateCaseRow), nil
		}
	}
	id := uuid.New()
	row := db.CreateCaseRow{CaseID: pgtype.UUID{Bytes: id, Valid: true}, TenantID: args.TenantID, Title: args.Title, Severity: "INFO", Status: string(workflow.NEW), Version: 1}
	f.cases[id.String()] = &caseState{status: string(workflow.NEW), version: 1}
	if args.IdemKey != "" {
		f.idem[args.IdemKey] = row
	}
	return row, nil
}

func (f *fakeService) Transition(ctx context.Context, args ports.TransitionArgs) (db.UpdateCaseStatusRow, error) {
	if args.IdemKey != "" {
		if v, ok := f.idem[args.IdemKey]; ok {
			return v.(db.UpdateCaseStatusRow), nil
		}
	}
	cs := f.cases[args.CaseID.String()]
	if args.IfMatch != 0 && cs.version != args.IfMatch {
		return db.UpdateCaseStatusRow{}, pgx.ErrNoRows
	}
	intent, err := workflow.Decide(workflow.State(cs.status), args.Event, args.Ctx, args.CaseID.String())
	if err != nil {
		return db.UpdateCaseStatusRow{}, err
	}
	cs.version++
	cs.status = string(intent.To)
	row := db.UpdateCaseStatusRow{CaseID: args.CaseID, Status: string(intent.To), Version: cs.version}
	if args.IdemKey != "" {
		f.idem[args.IdemKey] = row
	}
	return row, nil
}

func setupRouter() (*gin.Engine, *fakeService) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := newFakeService()
	RegisterRoutes(r, svc)
	return r, svc
}

func createCase(t *testing.T, r *gin.Engine) (string, int64) {
	reqBody := `{"tenant_id":1,"title":"t"}`
	req := httptest.NewRequest("POST", "/case/create", bytes.NewBufferString(reqBody))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create case status %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	id := resp["case_id"].(string)
	ver := int64(resp["version"].(float64))
	return id, ver
}

func TestIllegalTransition(t *testing.T) {
	r, _ := setupRouter()
	id, ver := createCase(t, r)
	body := `{"event":"exec_done"}`
	req := httptest.NewRequest("PATCH", "/case/"+id+"/transition", bytes.NewBufferString(body))
	req.Header.Set("If-Match", strconv.FormatInt(ver, 10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", w.Code)
	}
}

func TestIdempotentReplay(t *testing.T) {
	r, _ := setupRouter()
	id, ver := createCase(t, r)
	body := `{"event":"start_analysis"}`
	req := httptest.NewRequest("PATCH", "/case/"+id+"/transition", bytes.NewBufferString(body))
	req.Header.Set("If-Match", strconv.FormatInt(ver, 10))
	req.Header.Set("Idempotency-Key", "key1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("first transition status %d", w.Code)
	}
	var resp1 map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp1)
	ver2 := int64(resp1["version"].(float64))

	// replay
	req2 := httptest.NewRequest("PATCH", "/case/"+id+"/transition", bytes.NewBufferString(body))
	req2.Header.Set("If-Match", strconv.FormatInt(ver, 10))
	req2.Header.Set("Idempotency-Key", "key1")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("replay status %d", w2.Code)
	}
	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	verReplay := int64(resp2["version"].(float64))
	if verReplay != ver2 {
		t.Fatalf("expected version %d on replay, got %d", ver2, verReplay)
	}
}

func TestFailureParked(t *testing.T) {
	r, _ := setupRouter()
	id, ver := createCase(t, r)
	// move to ANALYZING
	body := `{"event":"start_analysis"}`
	req := httptest.NewRequest("PATCH", "/case/"+id+"/transition", bytes.NewBufferString(body))
	req.Header.Set("If-Match", strconv.FormatInt(ver, 10))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("start_analysis status %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	ver2 := int64(resp["version"].(float64))

	// send failure event
	body2 := `{"event":"analysis_failed"}`
	req2 := httptest.NewRequest("PATCH", "/case/"+id+"/transition", bytes.NewBufferString(body2))
	req2.Header.Set("If-Match", strconv.FormatInt(ver2, 10))
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("analysis_failed status %d", w2.Code)
	}
	var resp2 map[string]any
	json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2["status"].(string) != string(workflow.PARKED) {
		t.Fatalf("expected PARKED, got %s", resp2["status"])
	}
}
