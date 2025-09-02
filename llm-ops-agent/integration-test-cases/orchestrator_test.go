package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"
)

const baseURL = "http://localhost:8100"

// waitForServer polls the health endpoint until the service is ready.
func waitForServer(t *testing.T) {
	t.Helper()
	for i := 0; i < 20; i++ {
		resp, err := http.Get(baseURL + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			resp.Body.Close()
			return
		}
		if resp != nil {
			resp.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Skip("orchestrator service not reachable")
}

func createCase(t *testing.T, idem string) (id string, version int64) {
	body := []byte(`{"tenant_id":1,"title":"p95 spike"}`)
	req, err := http.NewRequest("POST", baseURL+"/case/create", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	req.Header.Set("X-Actor", "tester")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create case: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create case status %d", resp.StatusCode)
	}
	var out struct {
		CaseID  string `json:"case_id"`
		Status  string `json:"status"`
		Version int64  `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out.CaseID, out.Version
}

func transition(t *testing.T, id string, ver int64, event, idem string) (status string, version int64, code int) {
	body := []byte(fmt.Sprintf(`{"event":"%s"}`, event))
	req, err := http.NewRequest("PATCH", baseURL+"/case/"+id+"/transition", bytes.NewBuffer(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ver != 0 {
		req.Header.Set("If-Match", strconv.FormatInt(ver, 10))
	}
	if idem != "" {
		req.Header.Set("Idempotency-Key", idem)
	}
	req.Header.Set("X-Actor", "tester")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("transition: %v", err)
	}
	defer resp.Body.Close()
	code = resp.StatusCode
	if code == http.StatusOK {
		var out struct {
			Status  string `json:"status"`
			Version int64  `json:"version"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return out.Status, out.Version, code
	}
	return "", 0, code
}

func TestHealthz(t *testing.T) {
	waitForServer(t)
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		t.Fatalf("healthz request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status %d", resp.StatusCode)
	}
}

func TestCreateCaseIdempotent(t *testing.T) {
	waitForServer(t)
	id1, _ := createCase(t, "create-1")
	id2, _ := createCase(t, "create-1")
	if id1 != id2 {
		t.Fatalf("idempotent create expected same id, got %s and %s", id1, id2)
	}
}

func TestTransitionIdempotent(t *testing.T) {
	waitForServer(t)
	id, ver := createCase(t, "")
	status1, ver1, code := transition(t, id, ver, "start_analysis", "trans-1")
	if code != http.StatusOK {
		t.Fatalf("first transition code %d", code)
	}
	status2, ver2, code := transition(t, id, ver, "start_analysis", "trans-1")
	if code != http.StatusOK {
		t.Fatalf("replay code %d", code)
	}
	if status1 != status2 || ver1 != ver2 {
		t.Fatalf("idempotent transition mismatch")
	}
}

func TestIllegalTransition(t *testing.T) {
	waitForServer(t)
	id, ver := createCase(t, "")
	_, _, code := transition(t, id, ver, "plan_ready", "")
	if code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", code)
	}
}

func TestVersionConflict(t *testing.T) {
	waitForServer(t)
	id, _ := createCase(t, "")
	_, _, code := transition(t, id, 0, "start_analysis", "")
	if code != http.StatusPreconditionFailed {
		t.Fatalf("expected 412, got %d", code)
	}
}
