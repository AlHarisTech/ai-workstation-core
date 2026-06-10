package mcp

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
	mcprouter "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/router"
	mcptypes "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/tools/filesystem"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/tools/git"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/tools/github"
)

type IntegrationGateway struct {
	kernel *kernel.KernelEngine
	router *mcprouter.ToolRouter
}

func NewIntegrationGateway(ke *kernel.KernelEngine) *IntegrationGateway {
	ig := &IntegrationGateway{
		kernel: ke,
		router: mcprouter.NewToolRouter(),
	}

	ig.router.Register("filesystem", "read", filesystem.New())
	ig.router.Register("filesystem", "write", filesystem.New())
	ig.router.Register("filesystem", "list", filesystem.New())
	ig.router.Register("filesystem", "search", filesystem.New())
	ig.router.Register("git", "status", git.New("."))
	ig.router.Register("git", "diff", git.New("."))
	ig.router.Register("git", "log", git.New("."))
	ig.router.Register("git", "branch", git.New("."))
	ig.router.Register("github", "create_pr", github.New())
	ig.router.Register("github", "list_issues", github.New())
	ig.router.Register("github", "create_issue", github.New())

	return ig
}

func (ig *IntegrationGateway) Process(raw json.RawMessage) mcptypes.MCPResponse {
	start := time.Now()

	var req mcptypes.MCPRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return mcptypes.NewResponse("parse_error", "", false, nil, err.Error(), 0)
	}

	if req.Timestamp == 0 {
		req.Timestamp = time.Now().UnixMilli()
	}

	// Step 1: Synchronous session validation
	if req.SessionID == "" || req.ProjectID == "" {
		return mcptypes.NewResponse(req.ID, "", false, nil, "SESSION_INVALID: session_id and project_id required", time.Since(start).Milliseconds())
	}

	// Step 2: Kernel pipeline (async — fires and forgets for audit)
	kernelReq := fmt.Sprintf(`{"id":"%s","method":"tool.call","params":{"tool":"echo","arguments":{}},"session":{"session_id":"%s","project_id":"%s"}}`,
		req.ID, req.SessionID, req.ProjectID)
	_ = ig.kernel.Ingest(json.RawMessage(kernelReq))

	// Step 3: Tool routing + execution
	resp, err := ig.router.Route(req)
	if err != nil {
		return mcptypes.NewResponse(req.ID, resp.TraceID, false, nil, err.Error(), time.Since(start).Milliseconds())
	}

	resp.LatencyMS = time.Since(start).Milliseconds()
	return resp
}

func (ig *IntegrationGateway) ListTools() []map[string]any {
	tools := ig.router.ListTools()
	result := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		result = append(result, map[string]any{
			"tool":    t,
			"actions": ig.router.ListActions(t),
		})
	}
	return result
}

func toJSON(v map[string]any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
