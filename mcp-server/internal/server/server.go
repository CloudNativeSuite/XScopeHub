package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/xscopehub/mcp-server/internal/registry"
	"github.com/xscopehub/mcp-server/internal/types"
	"github.com/xscopehub/mcp-server/pkg/manifest"
)

// Options configures the MCP HTTP server.
type Options struct {
	Manifest     manifest.Manifest
	Registry     *registry.Registry
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// Server implements http.Handler for MCP JSON-RPC.
type Server struct {
	manifest manifest.Manifest
	registry *registry.Registry
}

// New creates a new MCP server instance.
func New(opts Options) *Server {
	return &Server{
		manifest: opts.Manifest,
		registry: opts.Registry,
	}
}

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "only POST supported", http.StatusMethodNotAllowed)
		return
	}

	defer func() {
		if err := r.Body.Close(); err != nil {
			log.Printf("failed to close body: %v", err)
		}
	}()

	payload, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("read body: %v", err), http.StatusBadRequest)
		return
	}

	var req Request
	if err := json.Unmarshal(payload, &req); err != nil {
		http.Error(w, fmt.Sprintf("decode request: %v", err), http.StatusBadRequest)
		return
	}

	resp := s.handleRequest(req)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("encode response: %v", err)
	}
}

// Request represents an MCP JSON-RPC request.
type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	Method  string           `json:"method"`
	Params  *json.RawMessage `json:"params"`
	ID      interface{}      `json:"id"`
}

// Response represents an MCP JSON-RPC response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   *Error      `json:"error,omitempty"`
}

// Error wraps JSON-RPC error payload.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) handleRequest(req Request) Response {
	switch req.Method {
	case "resources/list":
		return s.handleResourcesList(req)
	case "resources/get":
		return s.handleResourceGet(req)
	case "tools/list":
		return s.handleToolsList(req)
	case "tools/call":
		return s.handleToolsCall(req)
	case "manifest/get":
		return s.handleManifest(req)
	default:
		return errorResponse(req.ID, -32601, fmt.Sprintf("method %s not found", req.Method))
	}
}

func (s *Server) handleResourcesList(req Request) Response {
	resources := s.registry.ListResources()
	return Response{JSONRPC: "2.0", ID: req.ID, Result: resources}
}

func (s *Server) handleResourceGet(req Request) Response {
	var params struct {
		Name string `json:"name"`
	}
	if err := decodeParams(req.Params, &params); err != nil {
		return errorResponse(req.ID, err.Code, err.Message)
	}

	payload, err := s.registry.Resource(params.Name)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}
	return Response{JSONRPC: "2.0", ID: req.ID, Result: payload}
}

func (s *Server) handleToolsList(req Request) Response {
	tools := s.registry.ListTools()
	return Response{JSONRPC: "2.0", ID: req.ID, Result: tools}
}

func (s *Server) handleToolsCall(req Request) Response {
	var params struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	}
	if err := decodeParams(req.Params, &params); err != nil {
		return errorResponse(req.ID, err.Code, err.Message)
	}

	if params.Arguments == nil {
		params.Arguments = map[string]interface{}{}
	}

	result, err := s.registry.InvokeTool(params.Name, params.Arguments)
	if err != nil {
		return errorResponse(req.ID, -32000, err.Error())
	}
	return Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) handleManifest(req Request) Response {
	meta := struct {
		Manifest  manifest.Manifest          `json:"manifest"`
		Resources []types.ResourceDescriptor `json:"resources"`
		Tools     []types.ToolDescriptor     `json:"tools"`
	}{
		Manifest:  s.manifest,
		Resources: s.registry.ListResources(),
		Tools:     s.registry.ListTools(),
	}

	return Response{JSONRPC: "2.0", ID: req.ID, Result: meta}
}

func decodeParams(raw *json.RawMessage, v interface{}) *Error {
	if raw == nil {
		return &Error{Code: -32602, Message: "missing params"}
	}
	if err := json.Unmarshal(*raw, v); err != nil {
		return &Error{Code: -32602, Message: fmt.Sprintf("invalid params: %v", err)}
	}
	return nil
}

func errorResponse(id interface{}, code int, message string) Response {
	return Response{
		JSONRPC: "2.0",
		ID:      id,
		Error: &Error{
			Code:    code,
			Message: message,
		},
	}
}
