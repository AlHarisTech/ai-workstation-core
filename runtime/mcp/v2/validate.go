package mcpv2

import (
	"errors"
	"fmt"
	"strings"
)

var validActionTypes = map[ActionType]bool{
	ActionGit:        true,
	ActionFilesystem: true,
	ActionMemory:     true,
	ActionGitHub:     true,
	ActionFetch:      true,
	ActionContext7:   true,
	ActionPostgres:   true,
	ActionChromaDB:   true,
}

var validPriorities = map[string]bool{
	"low": true, "medium": true, "high": true, "critical": true,
}

var validAuthTypes = map[string]bool{
	"bearer": true, "api_key": true, "mcp_signed": true,
}

var maxTimeout = 120000
var maxRetry = 3

func ValidateRequest(req *MCPRequest) error {
	var errs []string

	if req.ID == "" {
		errs = append(errs, "id is required")
	}
	if req.Timestamp == "" {
		errs = append(errs, "timestamp is required")
	}
	if req.Source != "opencode" {
		errs = append(errs, "source must be 'opencode'")
	}
	if req.Type != "mcp.request" {
		errs = append(errs, "type must be 'mcp.request'")
	}

	// Action validation (Rule 1)
	if !validActionTypes[req.Action.Type] {
		errs = append(errs, fmt.Sprintf("invalid action type: %s", req.Action.Type))
	}
	if req.Action.Operation == "" {
		errs = append(errs, "action operation is required")
	}

	// Context integrity (Rule 3)
	if req.Context.SessionID == "" {
		errs = append(errs, "context.session_id is required")
	}
	if req.Context.TraceID == "" {
		errs = append(errs, "context.trace_id is required")
	}
	if req.Context.Workspace.Path == "" {
		errs = append(errs, "context.workspace.path is required")
	}

	// Auth validation
	if !validAuthTypes[req.Auth.Type] {
		errs = append(errs, fmt.Sprintf("invalid auth type: %s", req.Auth.Type))
	}

	// Meta validation (Rule 4)
	if !validPriorities[req.Meta.Priority] {
		errs = append(errs, fmt.Sprintf("invalid priority: %s", req.Meta.Priority))
	}
	if req.Meta.Timeout > maxTimeout {
		errs = append(errs, fmt.Sprintf("timeout_ms exceeds max %d", maxTimeout))
	}
	if req.Meta.Retry > maxRetry {
		errs = append(errs, fmt.Sprintf("retry exceeds max %d", maxRetry))
	}

	if len(errs) > 0 {
		return errors.New("validation failed: " + strings.Join(errs, "; "))
	}
	return nil
}
