package registry

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xscopehub/mcp-server/internal/types"
)

// Registry maintains the available resources and tools.
type Registry struct {
	resources    map[string]types.ResourceDescriptor
	resourceData map[string]types.ResourcePayload
	tools        map[string]ToolFunc
	toolInfo     map[string]types.ToolDescriptor
}

// ToolFunc represents the implementation of an MCP tool call.
type ToolFunc func(arguments map[string]interface{}) (types.ToolResult, error)

// New creates an empty registry.
func New() *Registry {
	return &Registry{
		resources:    make(map[string]types.ResourceDescriptor),
		resourceData: make(map[string]types.ResourcePayload),
		tools:        make(map[string]ToolFunc),
		toolInfo:     make(map[string]types.ToolDescriptor),
	}
}

// RegisterResources adds resource descriptors and payloads.
func (r *Registry) RegisterResources(resources []types.ResourcePayload) {
	for _, res := range resources {
		desc := types.ResourceDescriptor{
			Name:        res.Name,
			Title:       strings.Title(strings.ReplaceAll(res.Name, "_", " ")),
			Description: res.Description,
		}
		r.resources[res.Name] = desc
		r.resourceData[res.Name] = res
	}
}

// ListResources returns all resource descriptors.
func (r *Registry) ListResources() []types.ResourceDescriptor {
	descriptors := make([]types.ResourceDescriptor, 0, len(r.resources))
	for _, desc := range r.resources {
		descriptors = append(descriptors, desc)
	}
	return descriptors
}

// Resource returns the payload for a resource.
func (r *Registry) Resource(name string) (types.ResourcePayload, error) {
	payload, ok := r.resourceData[name]
	if !ok {
		return types.ResourcePayload{}, fmt.Errorf("resource %s not found", name)
	}
	return payload, nil
}

// RegisterTool registers a single tool implementation.
func (r *Registry) RegisterTool(desc types.ToolDescriptor, fn ToolFunc) {
	r.toolInfo[desc.Name] = desc
	r.tools[desc.Name] = fn
}

// RegisterTools registers multiple tool implementations.
func (r *Registry) RegisterTools(tools map[string]Tool) {
	for _, tool := range tools {
		r.RegisterTool(tool.Descriptor, tool.Func)
	}
}

// Tool describes a tool registration payload.
type Tool struct {
	Descriptor types.ToolDescriptor
	Func       ToolFunc
}

// ListTools returns tool descriptors.
func (r *Registry) ListTools() []types.ToolDescriptor {
	descriptors := make([]types.ToolDescriptor, 0, len(r.toolInfo))
	for _, desc := range r.toolInfo {
		descriptors = append(descriptors, desc)
	}
	return descriptors
}

// InvokeTool executes a tool by name.
func (r *Registry) InvokeTool(name string, arguments map[string]interface{}) (types.ToolResult, error) {
	fn, ok := r.tools[name]
	if !ok {
		return types.ToolResult{}, fmt.Errorf("tool %s not found", name)
	}
	if fn == nil {
		return types.ToolResult{}, errors.New("tool implementation missing")
	}
	return fn(arguments)
}

// StaticResources returns example resources for the skeleton server.
func StaticResources() []types.ResourcePayload {
	return []types.ResourcePayload{
		{
			Name:        "logs",
			Description: "Recent log events captured by XScopeHub",
			Data: []map[string]interface{}{
				{"timestamp": "2024-01-01T00:00:00Z", "service": "observe-gateway", "level": "info", "message": "startup complete"},
				{"timestamp": "2024-01-01T00:05:00Z", "service": "llm-ops-agent", "level": "warn", "message": "queue depth high"},
			},
		},
		{
			Name:        "metrics",
			Description: "Key service metrics aggregated from OpenObserve",
			Data: map[string]interface{}{
				"observe_bridge.latency_p95_ms": 120.0,
				"llm_ops_agent.queue_depth":     7,
				"vector.ingest_rate":            5480,
			},
		},
		{
			Name:        "traces",
			Description: "Representative traces with dependency context",
			Data: []map[string]interface{}{
				{
					"trace_id":       "84dd9a5c9f8c1f7b",
					"root_service":   "observe-gateway",
					"entry_span":     "HTTP GET /api/query",
					"duration_ms":    342,
					"status":         "ok",
					"critical_path":  []string{"observe-gateway", "observe-bridge", "openobserve"},
					"error_spans":    []interface{}{},
					"correlated_log": "2024-01-01T00:05:01Z",
				},
				{
					"trace_id":       "3b0fb66c41a2b3de",
					"root_service":   "llm-ops-agent",
					"entry_span":     "POST /v1/insight",
					"duration_ms":    1280,
					"status":         "error",
					"critical_path":  []string{"llm-ops-agent", "observe-bridge", "postgres"},
					"error_spans":    []string{"pg-writer"},
					"correlated_log": "2024-01-01T00:07:11Z",
				},
			},
		},
		{
			Name:        "topology",
			Description: "Service, network, and infrastructure relationships",
			Data: map[string]interface{}{
				"service": []map[string]interface{}{
					{"from": "observe-gateway", "to": "observe-bridge", "edge": "CALLS"},
					{"from": "observe-bridge", "to": "postgres", "edge": "WRITES"},
				},
				"deployment": []map[string]interface{}{
					{"service": "observe-gateway", "pod": "gw-67f6b", "node": "worker-a1", "rev": "sha256:abc"},
					{"service": "observe-bridge", "pod": "bridge-12ea4", "node": "worker-b4", "rev": "sha256:def"},
				},
				"network": []map[string]interface{}{
					{"source": "gw-67f6b", "dest": "bridge-12ea4", "flow_per_s": 2150},
					{"source": "bridge-12ea4", "dest": "postgres", "flow_per_s": 480},
				},
				"infrastructure": []map[string]interface{}{
					{"node": "worker-a1", "subnet": "10.0.1.0/24", "zone": "us-east-1a", "account": "prod"},
					{"node": "worker-b4", "subnet": "10.0.2.0/24", "zone": "us-east-1b", "account": "prod"},
				},
			},
		},
		{
			Name:        "knowledge",
			Description: "Contextual knowledge spanning alerts, incidents, configs, and runbooks",
			Data: map[string]interface{}{
				"alerts": []map[string]interface{}{
					{"id": "alert-ops-1", "severity": "critical", "summary": "OpenObserve ingestion stalled", "trace_id": "3b0fb66c41a2b3de"},
					{"id": "alert-ops-2", "severity": "warning", "summary": "Vector agent backpressure", "node": "worker-b4"},
				},
				"incidents": []map[string]interface{}{
					{"id": "inc-20240101-1", "status": "mitigated", "primary_service": "observe-bridge", "started_at": "2024-01-01T00:06:00Z", "ended_at": "2024-01-01T00:20:00Z"},
				},
				"runbooks": []map[string]interface{}{
					{"id": "rb-ob-restore", "title": "Restore ObserveBridge ingestion", "service": "observe-bridge", "link": "https://wiki.example.com/runbooks/ob-restore"},
				},
				"config": []map[string]interface{}{
					{"id": "cfg-gateway-20240101", "component": "observe-gateway", "change": "Increased rate limit to 20rps", "git_ref": "main@c1d2e3"},
				},
				"change_records": []map[string]interface{}{
					{"id": "chg-221", "submitted_by": "sre-bot", "summary": "Rolled out vector agent 0.28", "window": "2024-01-01T00:00:00Z/2024-01-01T00:30:00Z"},
				},
				"cmdb": []map[string]interface{}{
					{"id": "svc-observe-bridge", "owner": "platform", "tier": "critical", "dependencies": []string{"postgres", "openobserve"}},
				},
			},
		},
	}
}

// StaticTools returns stub tool implementations.
func StaticTools() map[string]Tool {
	return map[string]Tool{
		"query_logs": {
			Descriptor: types.ToolDescriptor{
				Name:        "query_logs",
				Description: "Filter logs by service name and severity.",
				InputSchema: "{\"service\":\"string\",\"level\":\"string\"}",
			},
			Func: func(arguments map[string]interface{}) (types.ToolResult, error) {
				service, _ := arguments["service"].(string)
				level, _ := arguments["level"].(string)
				result := fmt.Sprintf("queried logs for service=%s level=%s", service, level)
				return types.ToolResult{Name: "query_logs", Output: map[string]string{"result": result}}, nil
			},
		},
		"summarize_alerts": {
			Descriptor: types.ToolDescriptor{
				Name:        "summarize_alerts",
				Description: "Summarize active alerts for operator review.",
				InputSchema: "{}",
			},
			Func: func(arguments map[string]interface{}) (types.ToolResult, error) {
				_ = arguments
				summary := "2 alerts active: 1 critical (OpenObserve ingestion stalled), 1 warning (Vector agent backpressure)."
				return types.ToolResult{Name: "summarize_alerts", Output: map[string]string{"summary": summary}}, nil
			},
		},
	}
}
