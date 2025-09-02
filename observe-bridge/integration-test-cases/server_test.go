package integration_test

import (
	"net/http"
	"os"
	"testing"
)

func baseURL() string {
	if u := os.Getenv("OBSERVE_BRIDGE_URL"); u != "" {
		return u
	}
	return "http://localhost:8080"
}

func TestHealthz(t *testing.T) {
	url := baseURL() + "/healthz"
	resp, err := http.Get(url)
	if err != nil {
		t.Skipf("service not available: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func TestMetricsAndOpenAPI(t *testing.T) {
	base := baseURL()
	if resp, err := http.Get(base + "/healthz"); err != nil || resp.StatusCode != http.StatusOK {
		if err == nil {
			resp.Body.Close()
		}
		t.Skip("service not running")
	} else {
		resp.Body.Close()
	}

	endpoints := []string{"/metrics", "/openapi.yaml"}
	for _, ep := range endpoints {
		resp, err := http.Get(base + ep)
		if err != nil {
			t.Fatalf("request %s failed: %v", ep, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", ep, resp.StatusCode)
		}
		resp.Body.Close()
	}
}
