package integration_test

import (
	"io"
	"net/http"
	"testing"
	"time"
)

// TestOOStream verifies that the /oo/stream endpoint streams data.
func TestOOStream(t *testing.T) {
	base := baseURL()
	client := http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(base + "/oo/stream?tenant=demo&from=0&to=1")
	if err != nil {
		t.Skipf("service not available: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	buf := make([]byte, 1)
	if _, err := resp.Body.Read(buf); err != nil && err != io.EOF {
		t.Fatalf("failed to read stream: %v", err)
	}
}
