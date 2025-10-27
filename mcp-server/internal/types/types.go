package types

// ResourceDescriptor describes a resource exposed by the MCP server.
type ResourceDescriptor struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// ResourcePayload represents resource data returned to clients.
type ResourcePayload struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Data        interface{} `json:"data"`
}

// ToolDescriptor describes a tool callable via the MCP server.
type ToolDescriptor struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema string `json:"input_schema"`
}

// ToolResult encapsulates tool execution output.
type ToolResult struct {
	Name   string      `json:"name"`
	Output interface{} `json:"output"`
}
