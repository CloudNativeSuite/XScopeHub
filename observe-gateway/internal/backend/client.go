package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xscopehub/observe-gateway/internal/config"
	"github.com/xscopehub/observe-gateway/internal/query"
)

// Client aggregates integrations with the different OpenObserve APIs.
type Client struct {
	oo                *openObserveClient
	fallback          *promFallbackClient
	metadata          *metadataStore
	defaultLogTable   string
	defaultTraceTable string
}

// New creates a backend client based on configuration.
func New(ctx context.Context, cfg config.BackendConfig) (*Client, error) {
	oo, err := newOpenObserveClient(cfg.OpenObserve)
	if err != nil {
		return nil, err
	}

	var fb *promFallbackClient
	if cfg.Fallback.Enabled {
		fb, err = newPromFallbackClient(cfg.Fallback)
		if err != nil {
			return nil, err
		}
	}

	metadataStore, err := newMetadataStore(ctx, cfg.Metadata)
	if err != nil {
		return nil, err
	}

	client := &Client{
		oo:                oo,
		fallback:          fb,
		metadata:          metadataStore,
		defaultLogTable:   cfg.OpenObserve.LogTable,
		defaultTraceTable: cfg.OpenObserve.TraceTable,
	}

	if client.defaultLogTable == "" {
		client.defaultLogTable = "logs"
	}
	if client.defaultTraceTable == "" {
		client.defaultTraceTable = "traces"
	}

	return client, nil
}

// QueryPromQL dispatches a PromQL request to OpenObserve with optional fallback.
func (c *Client) QueryPromQL(ctx context.Context, tenant string, req query.Request) (Result, error) {
	meta, err := c.resolveTenantMetadata(ctx, tenant)
	if err != nil {
		return Result{}, err
	}

	res, err := c.oo.queryPromQL(ctx, meta.Org, tenant, req)
	if err == nil {
		res.Backend = "openobserve-promql"
		return res, nil
	}

	var unsupported *UnsupportedError
	if errors.As(err, &unsupported) && c.fallback != nil {
		fbRes, fbErr := c.fallback.queryPromQL(ctx, tenant, req)
		if fbErr == nil {
			fbRes.Backend = "fallback-promql"
			return fbRes, nil
		}
		if fbErr != nil {
			return Result{}, fbErr
		}
	}

	return Result{}, err
}

// QueryLogQL handles LogQL by translating into SQL and invoking OpenObserve search.
func (c *Client) QueryLogQL(ctx context.Context, tenant string, req query.Request) (Result, error) {
	meta, err := c.resolveTenantMetadata(ctx, tenant)
	if err != nil {
		return Result{}, err
	}

	sql, err := translateLogQL(req.Query, meta.LogTable)
	if err != nil {
		return Result{}, err
	}

	body := map[string]any{
		"sql":    sql,
		"start":  req.Start,
		"end":    req.End,
		"tenant": tenant,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}

	res, err := c.oo.postJSON(ctx, tenant, c.oo.logSearchURL(meta.Org), payload)
	if err != nil {
		return Result{}, err
	}

	res.Backend = "openobserve-logsql"
	return res, nil
}

// QueryTraceQL handles TraceQL translations.
func (c *Client) QueryTraceQL(ctx context.Context, tenant string, req query.Request) (Result, error) {
	meta, err := c.resolveTenantMetadata(ctx, tenant)
	if err != nil {
		return Result{}, err
	}

	sql, err := translateTraceQL(req.Query, meta.TraceTable)
	if err != nil {
		return Result{}, err
	}

	body := map[string]any{
		"sql":    sql,
		"start":  req.Start,
		"end":    req.End,
		"tenant": tenant,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return Result{}, err
	}

	res, err := c.oo.postJSON(ctx, tenant, c.oo.traceSearchURL(meta.Org), payload)
	if err != nil {
		return Result{}, err
	}

	res.Backend = "openobserve-tracesql"
	return res, nil
}

// Close releases any backend resources.
func (c *Client) Close() {
	if c.metadata != nil {
		c.metadata.Close()
	}
}

func (c *Client) resolveTenantMetadata(ctx context.Context, tenant string) (tenantMetadata, error) {
	meta := tenantMetadata{
		Org:        c.oo.defaultOrg,
		LogTable:   c.defaultLogTable,
		TraceTable: c.defaultTraceTable,
	}

	if c.metadata == nil {
		return meta, nil
	}

	tenantMeta, err := c.metadata.Lookup(ctx, tenant)
	if err != nil {
		if errors.Is(err, errTenantNotFound) {
			return meta, nil
		}
		return tenantMetadata{}, err
	}

	if tenantMeta.Org != "" {
		meta.Org = tenantMeta.Org
	}
	if tenantMeta.LogTable != "" {
		meta.LogTable = tenantMeta.LogTable
	}
	if tenantMeta.TraceTable != "" {
		meta.TraceTable = tenantMeta.TraceTable
	}

	return meta, nil
}

// ---- OpenObserve client implementation ----

type openObserveClient struct {
	baseURL     *url.URL
	defaultOrg  string
	apiKey      string
	http        *http.Client
	promQuery   string
	promRange   string
	logSearch   string
	traceSearch string
}

func newOpenObserveClient(cfg config.OpenObserveConfig) (*openObserveClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("openobserve base_url required")
	}

	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse base_url: %w", err)
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	return &openObserveClient{
		baseURL:     parsed,
		defaultOrg:  cfg.Org,
		apiKey:      cfg.APIKey,
		http:        &http.Client{Timeout: timeout},
		promQuery:   cfg.PromQueryEndpoint,
		promRange:   cfg.PromRangeEndpoint,
		logSearch:   cfg.LogSearchEndpoint,
		traceSearch: cfg.TraceSearchEndpoint,
	}, nil
}

func (c *openObserveClient) promURL(org string, isRange bool) (string, error) {
	endpoint := c.promQuery
	if isRange {
		endpoint = c.promRange
	}
	if endpoint == "" {
		return "", fmt.Errorf("promql endpoint not configured")
	}
	rel := endpoint
	resolvedOrg := c.resolveOrg(org)
	if strings.Contains(endpoint, "%s") {
		rel = fmt.Sprintf(endpoint, resolvedOrg)
	}
	return c.resolve(rel), nil
}

func (c *openObserveClient) logSearchURL(org string) string {
	endpoint := c.logSearch
	resolvedOrg := c.resolveOrg(org)
	if strings.Contains(endpoint, "%s") {
		endpoint = fmt.Sprintf(endpoint, resolvedOrg)
	}
	return c.resolve(endpoint)
}

func (c *openObserveClient) traceSearchURL(org string) string {
	endpoint := c.traceSearch
	resolvedOrg := c.resolveOrg(org)
	if strings.Contains(endpoint, "%s") {
		endpoint = fmt.Sprintf(endpoint, resolvedOrg)
	}
	return c.resolve(endpoint)
}

func (c *openObserveClient) resolveOrg(org string) string {
	if org != "" {
		return org
	}
	return c.defaultOrg
}

func (c *openObserveClient) resolve(rel string) string {
	if strings.HasPrefix(rel, "http") {
		return rel
	}
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, rel)
	return u.String()
}

func (c *openObserveClient) queryPromQL(ctx context.Context, org, tenant string, req query.Request) (Result, error) {
	isRange := req.HasTimeRange()
	endpoint, err := c.promURL(org, isRange)
	if err != nil {
		return Result{}, err
	}

	q := endpoint
	u, err := url.Parse(endpoint)
	if err == nil {
		params := u.Query()
		params.Set("query", req.Query)
		if isRange {
			params.Set("start", strconv.FormatFloat(float64(req.Start.UnixNano())/1e9, 'f', -1, 64))
			params.Set("end", strconv.FormatFloat(float64(req.End.UnixNano())/1e9, 'f', -1, 64))
			if step, err := req.StepDuration(); err == nil && step > 0 {
				params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
			}
		} else {
			params.Set("time", strconv.FormatFloat(float64(time.Now().UnixNano())/1e9, 'f', -1, 64))
		}
		u.RawQuery = params.Encode()
		q = u.String()
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, q, nil)
	if err != nil {
		return Result{}, err
	}
	c.applyHeaders(httpReq, tenant)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	if resp.StatusCode >= 400 {
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusNotImplemented {
			return Result{}, &UnsupportedError{Status: resp.StatusCode, Message: string(body)}
		}
		return Result{}, fmt.Errorf("openobserve promql error: %s", string(body))
	}

	return Result{
		Payload: json.RawMessage(body),
		Cost:    parseCost(resp.Header),
	}, nil
}

func (c *openObserveClient) postJSON(ctx context.Context, tenant, url string, payload []byte) (Result, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	c.applyHeaders(httpReq, tenant)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	if resp.StatusCode >= 400 {
		return Result{}, fmt.Errorf("openobserve error: %s", string(body))
	}

	return Result{Payload: json.RawMessage(body), Cost: parseCost(resp.Header)}, nil
}

func (c *openObserveClient) applyHeaders(req *http.Request, tenant string) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if tenant != "" {
		req.Header.Set("X-Tenant", tenant)
	}
}

// ---- Fallback PromQL client ----

type promFallbackClient struct {
	baseURL   *url.URL
	http      *http.Client
	queryPath string
	rangePath string
	apiKey    string
}

func newPromFallbackClient(cfg config.FallbackConfig) (*promFallbackClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("fallback base_url required")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse fallback base_url: %w", err)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &promFallbackClient{
		baseURL:   parsed,
		http:      &http.Client{Timeout: timeout},
		queryPath: cfg.QueryEndpoint,
		rangePath: cfg.RangeEndpoint,
		apiKey:    cfg.APIKey,
	}, nil
}

func (c *promFallbackClient) promURL(isRange bool) (string, error) {
	endpoint := c.queryPath
	if isRange {
		endpoint = c.rangePath
	}
	if endpoint == "" {
		endpoint = "/api/v1/query"
		if isRange {
			endpoint = "/api/v1/query_range"
		}
	}
	if strings.HasPrefix(endpoint, "http") {
		return endpoint, nil
	}
	u := *c.baseURL
	u.Path = path.Join(c.baseURL.Path, endpoint)
	return u.String(), nil
}

func (c *promFallbackClient) queryPromQL(ctx context.Context, tenant string, req query.Request) (Result, error) {
	isRange := req.HasTimeRange()
	endpoint, err := c.promURL(isRange)
	if err != nil {
		return Result{}, err
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return Result{}, err
	}
	params := u.Query()
	params.Set("query", req.Query)
	if isRange {
		params.Set("start", strconv.FormatFloat(float64(req.Start.UnixNano())/1e9, 'f', -1, 64))
		params.Set("end", strconv.FormatFloat(float64(req.End.UnixNano())/1e9, 'f', -1, 64))
		if step, err := req.StepDuration(); err == nil && step > 0 {
			params.Set("step", strconv.FormatFloat(step.Seconds(), 'f', -1, 64))
		}
	}
	u.RawQuery = params.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Result{}, err
	}
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if tenant != "" {
		httpReq.Header.Set("X-Tenant", tenant)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, err
	}

	if resp.StatusCode >= 400 {
		return Result{}, fmt.Errorf("fallback promql error: %s", string(body))
	}

	return Result{Payload: json.RawMessage(body), Cost: parseCost(resp.Header)}, nil
}

func parseCost(h http.Header) int64 {
	if h == nil {
		return 0
	}
	val := h.Get("X-Query-Cost")
	if val == "" {
		val = h.Get("X-O2-Query-Cost")
	}
	if val == "" {
		return 0
	}
	cost, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0
	}
	return cost
}

// --- Translators ---

func translateLogQL(q, table string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("empty logql")
	}

	table = sanitizeSQLIdentifier(table)
	if table == "" {
		table = "logs"
	}

	var conditions []string
	if strings.HasPrefix(q, "{") {
		idx := strings.Index(q, "}")
		if idx == -1 {
			return "", fmt.Errorf("invalid logql selector")
		}
		selector := q[1:idx]
		q = strings.TrimSpace(q[idx+1:])
		if selector != "" {
			parts := strings.Split(selector, ",")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				switch {
				case strings.Contains(part, "!="):
					kv := strings.SplitN(part, "!=", 2)
					if len(kv) != 2 {
						continue
					}
					conditions = append(conditions, fmt.Sprintf("labels->>'%s' <> '%s'", sanitizeSQLIdentifier(kv[0]), escapeSQLValue(kv[1])))
				case strings.Contains(part, "="):
					kv := strings.SplitN(part, "=", 2)
					if len(kv) != 2 {
						continue
					}
					conditions = append(conditions, fmt.Sprintf("labels->>'%s' = '%s'", sanitizeSQLIdentifier(kv[0]), escapeSQLValue(kv[1])))
				}
			}
		}
	}

	matches := logPipelineRegex.FindAllStringSubmatch(q, -1)
	for _, m := range matches {
		op := m[1]
		val := escapeSQLValue(m[2])
		switch op {
		case "=":
			conditions = append(conditions, fmt.Sprintf("message ILIKE '%%%s%%'", val))
		case "!=":
			conditions = append(conditions, fmt.Sprintf("message NOT ILIKE '%%%s%%'", val))
		case "~":
			conditions = append(conditions, fmt.Sprintf("message ~ '%s'", val))
		case "!~":
			conditions = append(conditions, fmt.Sprintf("message !~ '%s'", val))
		}
	}

	if len(conditions) == 0 {
		conditions = append(conditions, "1=1")
	}

	return fmt.Sprintf("SELECT * FROM %s WHERE %s", table, strings.Join(conditions, " AND ")), nil
}

func translateTraceQL(q, table string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", fmt.Errorf("empty traceql")
	}

	table = sanitizeSQLIdentifier(table)
	if table == "" {
		table = "traces"
	}

	lower := strings.ToLower(q)
	if !strings.HasPrefix(lower, "from") {
		return "", fmt.Errorf("traceql must start with FROM")
	}

	tokens := strings.Fields(q)
	if len(tokens) < 2 {
		return "", fmt.Errorf("traceql missing stream")
	}
	stream := tokens[1]
	conditions := []string{fmt.Sprintf("trace_stream = '%s'", escapeSQLValue(stream))}

	whereIdx := strings.Index(lower, " where ")
	if whereIdx != -1 {
		condExpr := strings.TrimSpace(q[whereIdx+7:])
		if condExpr != "" {
			parts := strings.Split(condExpr, " and ")
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" {
					continue
				}
				switch {
				case strings.Contains(part, "!="):
					kv := strings.SplitN(part, "!=", 2)
					conditions = append(conditions, fmt.Sprintf("attributes->>'%s' <> '%s'", sanitizeSQLIdentifier(kv[0]), escapeSQLValue(kv[1])))
				case strings.Contains(part, "="):
					kv := strings.SplitN(part, "=", 2)
					conditions = append(conditions, fmt.Sprintf("attributes->>'%s' = '%s'", sanitizeSQLIdentifier(kv[0]), escapeSQLValue(kv[1])))
				case strings.Contains(part, ">"):
					kv := strings.SplitN(part, ">", 2)
					conditions = append(conditions, fmt.Sprintf("%s > %s", sanitizeSQLIdentifier(kv[0]), strings.TrimSpace(kv[1])))
				case strings.Contains(part, "<"):
					kv := strings.SplitN(part, "<", 2)
					conditions = append(conditions, fmt.Sprintf("%s < %s", sanitizeSQLIdentifier(kv[0]), strings.TrimSpace(kv[1])))
				}
			}
		}
	}

	return fmt.Sprintf("SELECT * FROM %s WHERE %s", table, strings.Join(conditions, " AND ")), nil
}

func sanitizeSQLIdentifier(in string) string {
	in = strings.TrimSpace(in)
	in = strings.Trim(in, "\"`'")
	in = strings.ReplaceAll(in, " ", "_")
	return in
}

func escapeSQLValue(in string) string {
	in = strings.TrimSpace(in)
	in = strings.Trim(in, "\"`'")
	return strings.ReplaceAll(in, "'", "''")
}

var logPipelineRegex = regexp.MustCompile(`\|([=!~]{1,2})\s*"([^"]*)"`)
