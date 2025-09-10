package oo

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/xscopehub/xscopehub/etl/pkg/window"
)

// Record represents a generic OpenObserve record.
type Record map[string]any

// Stream reads logs, metrics, and traces for the tenant in the given window and invokes fn for each record.
// It queries the OpenObserve OTEL HTTP API for each data type and streams NDJSON results.
func Stream(ctx context.Context, tenant string, w window.Window, fn func(Record)) error {
	endpoint := os.Getenv("OPENOBSERVE_URL")
	if endpoint == "" {
		return fmt.Errorf("OPENOBSERVE_URL not set")
	}
	auth := os.Getenv("OPENOBSERVE_AUTH")
	client := http.Client{}
	types := []string{"logs", "metrics", "traces"}
	for _, typ := range types {
		url := fmt.Sprintf("%s%s/_search?start=%d&end=%d", endpoint, typ, w.From.UnixMilli(), w.To.UnixMilli())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		req.Header.Set("Accept", "application/x-ndjson")
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			var rec Record
			if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
				continue
			}
			rec["type"] = typ
			if tenant != "" {
				rec["tenant"] = tenant
			}
			fn(rec)
		}
		resp.Body.Close()
	}
	return nil
}
