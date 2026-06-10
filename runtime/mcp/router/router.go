package router

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type ToolRouter struct {
	mu      sync.RWMutex
	routes  map[string]map[string]types.MCPAdapter
}

func NewToolRouter() *ToolRouter {
	return &ToolRouter{
		routes: make(map[string]map[string]types.MCPAdapter),
	}
}

func (tr *ToolRouter) Register(tool, action string, adapter types.MCPAdapter) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	if _, ok := tr.routes[tool]; !ok {
		tr.routes[tool] = make(map[string]types.MCPAdapter)
	}
	tr.routes[tool][action] = adapter
}

var routeCounter uint64

func (tr *ToolRouter) Route(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	actions, ok := tr.routes[req.Tool]
	if !ok {
		return types.MCPResponse{
			ID: req.ID, Success: false,
			Error: fmt.Sprintf("no adapter registered for tool: %s", req.Tool),
		}, fmt.Errorf("tool not found: %s", req.Tool)
	}

	adapter, ok := actions[req.Action]
	if !ok {
		return types.MCPResponse{
			ID: req.ID, Success: false,
			Error: fmt.Sprintf("no adapter for action: %s.%s", req.Tool, req.Action),
		}, fmt.Errorf("action not found: %s", req.Action)
	}

	start := time.Now()
	resp, err := adapter.Execute(ctx, req)
	latency := time.Since(start).Milliseconds()
	resp.LatencyMS = latency
	resp.TraceID = fmt.Sprintf("tr_%s_%s_%s_%d", req.Tool, req.Action, req.ID, routeCounter)
	routeCounter++
	return resp, err
}

func (tr *ToolRouter) ListTools() []string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	tools := make([]string, 0, len(tr.routes))
	for tool := range tr.routes {
		tools = append(tools, tool)
	}
	return tools
}

func (tr *ToolRouter) ListActions(tool string) []string {
	tr.mu.RLock()
	defer tr.mu.RUnlock()
	if actions, ok := tr.routes[tool]; ok {
		result := make([]string, 0, len(actions))
		for action := range actions {
			result = append(result, action)
		}
		return result
	}
	return nil
}
