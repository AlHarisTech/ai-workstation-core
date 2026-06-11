package mcpv2

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"time"
)

type Gateway struct {
	router *Router
	policy *PolicyEngine
	servers map[string]MCPServer
}

func NewGateway() *Gateway {
	g := &Gateway{
		router:  NewRouter(),
		policy:  NewPolicyEngine(),
		servers: make(map[string]MCPServer),
	}
	g.registerDefaults()
	return g
}

func (g *Gateway) registerDefaults() {
	g.servers["git"] = &GitServer{}
	g.servers["filesystem"] = &FilesystemServer{}
	g.servers["memory"] = NewMemoryServer("")
	g.servers["github"] = NewGitHubServer()
	g.servers["fetch"] = &FetchServer{}
	g.servers["context7"] = NewContext7Server()
	g.servers["postgres"] = NewPostgresAdapter()
	g.servers["chroma"] = NewChromaAdapter()
	g.servers["chromadb"] = g.servers["chroma"]
}

func (g *Gateway) Server(name string) MCPServer {
	if s, ok := g.servers[name]; ok {
		return s
	}
	return nil
}

func (g *Gateway) RegisterServer(s MCPServer) {
	g.servers[s.Name()] = s
}

func (g *Gateway) Process(req *MCPRequest) *MCPResponse {
	start := time.Now()
	// Ensure trace IDs are available before any processing
	if req.Meta.TraceID == "" {
		req.Meta.TraceID = GenerateTraceID()
	}
	if req.Meta.SpanID == "" {
		req.Meta.SpanID = GenerateSpanID()
	}

	resp := &MCPResponse{
		ID:        req.ID,
		RequestID: req.ID,
		Status:    "success",
		Execution: ExecutionResult{
			Operation: string(req.Action.Type) + "." + req.Action.Operation,
		},
		Result: ResultData{Format: "json"},
		Meta: ResponseMeta{
			TraceID: req.Meta.TraceID,
			SpanID:  req.Meta.SpanID,
		},
	}

	// Step 1: Validate request schema
	if err := ValidateRequest(req); err != nil {
		return errorResponse(resp, "VALIDATION_ERROR", err.Error(), false)
	}

	// Step 2: Enforce policy (fail closed)
	if err := g.policy.Enforce(req.Action.Type, req.Action.Operation, req.Policy); err != nil {
		return errorResponse(resp, "POLICY_DENIED", err.Error(), false)
	}

	// Step 3: Resolve capability → server mapping
	cap, err := g.router.Resolve(req.Action.Type, req.Action.Operation)
	if err != nil {
		return errorResponse(resp, "ROUTE_NOT_FOUND", err.Error(), false)
	}

	// Step 4: Route to MCP server
	server, ok := g.servers[cap.Server]
	if !ok {
		return errorResponse(resp, "SERVER_NOT_FOUND", "no server registered: "+cap.Server, false)
	}

	resp.Execution.Server = cap.Server

	// Step 5: Execute MCP call
	req.Context.TimeoutMs = req.Meta.Timeout
	result, execErr := server.Execute(string(req.Action.Type)+"."+req.Action.Operation, req.Payload.Parameters, req.Context)
	resp.Execution.Duration = time.Since(start).Milliseconds()

	if execErr != nil {
		return errorResponse(resp, "EXECUTION_FAILED", execErr.Error(), true)
	}

	// Step 6: Normalize response
	resp.Result.Data = result
	resp.Result.Format = "json"

	return resp
}

func errorResponse(resp *MCPResponse, code, message string, recoverable bool) *MCPResponse {
	resp.Status = "error"
	resp.Error = ErrorInfo{
		Code:        code,
		Message:     message,
		Recoverable: recoverable,
	}
	return resp
}

func (g *Gateway) Listen() *MCPRequest {
	var req MCPRequest
	err := json.NewDecoder(os.Stdin).Decode(&req)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		log.Fatalf("[GATEWAY] failed to read request: %v", err)
	}
	return &req
}
