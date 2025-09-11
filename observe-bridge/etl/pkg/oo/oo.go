package oo

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"

	"golang.org/x/net/http2"

	"github.com/xscopehub/xscopehub/etl/pkg/window"
)

// Record represents a generic OpenObserve record.
type Record map[string]any

// Stream reads logs, metrics, and traces for the tenant in the given window and invokes fn for each record.
// When stream is true, it uses OpenObserve's streaming search via /_search_stream over a single HTTP/2 connection.
// Otherwise, it performs a one-off search via /_search.
// Both modes rely on SQL with microsecond timestamps.
func Stream(ctx context.Context, endpoint string, headers map[string]string, tenant string, w window.Window, stream bool, fn func(Record)) error {
	if endpoint == "" {
		return fmt.Errorf("openobserve endpoint not set")
	}

	transport := &http2.Transport{
		AllowHTTP: true,
		DialTLS: func(network, addr string, cfg *tls.Config) (net.Conn, error) {
			return net.Dial(network, addr)
		},
	}
	client := http.Client{Transport: transport}

	types := []string{"logs", "metrics", "traces"}
	path := "_search"
	if stream {
		path = "_search_stream"
	}
	for _, typ := range types {
		sql := fmt.Sprintf("SELECT * FROM \"%s\" WHERE _timestamp >= %d AND _timestamp < %d", typ, w.From.UnixMicro(), w.To.UnixMicro())
		body, _ := json.Marshal(map[string]string{"sql": sql})
		url := fmt.Sprintf("%s%s/%s", endpoint, typ, path)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		req.Header.Set("Content-Type", "application/json")
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
