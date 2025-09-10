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
func Stream(ctx context.Context, endpoint string, headers map[string]string, tenant string, w window.Window, fn func(Record)) error {
	if endpoint == "" {
		return fmt.Errorf("openobserve endpoint not set")
	}
	client := http.Client{}
	types := []string{"logs", "metrics", "traces"}
	for _, typ := range types {
		url := fmt.Sprintf("%s%s/_search?start=%d&end=%d", endpoint, typ, w.From.UnixMilli(), w.To.UnixMilli())
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
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
