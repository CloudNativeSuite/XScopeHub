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
			Name:        "alerts",
			Description: "Active alerts across the platform",
			Data: []map[string]interface{}{
				{"id": "alert-ops-1", "severity": "critical", "summary": "OpenObserve ingestion stalled"},
				{"id": "alert-ops-2", "severity": "warning", "summary": "Vector agent backpressure"},
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
