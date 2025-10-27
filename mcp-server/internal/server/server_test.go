package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/xscopehub/mcp-server/internal/registry"
	"github.com/xscopehub/mcp-server/pkg/manifest"
)

func TestServeHTTPResourcesList(t *testing.T) {
	mf := manifest.Manifest{Name: "xscopehub"}
	reg := registry.New()
	reg.RegisterResources(registry.StaticResources())
	reg.RegisterTools(registry.StaticTools())
	srv := New(Options{Manifest: mf, Registry: reg})

	payload := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "resources/list",
		"id":      1,
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	res := httptest.NewRecorder()

	srv.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", res.Code)
	}

	var resp Response
	if err := json.Unmarshal(res.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp.Error != nil {
		t.Fatalf("unexpected error response: %+v", resp.Error)
	}

	if resp.Result == nil {
		t.Fatalf("expected result payload")
	}
}
