package policy

import (
	"path/filepath"
	"strings"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

func DefaultRules() []PolicyRule {
	return []PolicyRule{
		{
			ID: "POL-001", Name: "mandatory-context-fields",
			Stage: "pre_validation", Priority: 10,
			Evaluate: ruleMandatoryFields,
		},
		{
			ID: "POL-002", Name: "tool-registry-existence",
			Stage: "capability_routing", Priority: 20,
			Evaluate: ruleToolExists,
		},
		{
			ID: "POL-003", Name: "session-gate",
			Stage: "session_guard", Priority: 30,
			Evaluate: ruleSessionGate,
		},
		{
			ID: "POL-004", Name: "path-access-control",
			Stage: "pre_execution", Priority: 40,
			Evaluate: rulePathDeny,
		},
		{
			ID: "POL-005", Name: "execution-timeout",
			Stage: "execution", Priority: 50,
			Evaluate: ruleTimeout,
		},
		{
			ID: "POL-006", Name: "error-isolation",
			Stage: "post_validation", Priority: 60,
			Evaluate: ruleErrorIsolation,
		},
	}
}

func ruleMandatoryFields(ctx *types.RequestContext, _ *types.ToolDef) types.PolicyVerdict {
	if ctx.RequestID == "" || ctx.SessionID == "" || ctx.ProjectID == "" {
		return types.PolicyVerdict{Decision: "DENY", Reason: "missing mandatory fields"}
	}
	return types.PolicyVerdict{Decision: "ALLOW"}
}

func ruleToolExists(ctx *types.RequestContext, toolDef *types.ToolDef) types.PolicyVerdict {
	if toolDef == nil && ctx.ToolID != "" {
		return types.PolicyVerdict{Decision: "DENY", Reason: "tool not in registry: " + ctx.ToolID}
	}
	return types.PolicyVerdict{Decision: "ALLOW"}
}

func ruleSessionGate(ctx *types.RequestContext, toolDef *types.ToolDef) types.PolicyVerdict {
	if toolDef == nil {
		return types.PolicyVerdict{Decision: "ALLOW"}
	}
	if toolDef.Governance.RequireSession {
		if ctx.SessionID == "" || ctx.ProjectID == "" {
			return types.PolicyVerdict{Decision: "DENY", Reason: "tool requires valid session"}
		}
	}
	return types.PolicyVerdict{Decision: "ALLOW"}
}

func rulePathDeny(ctx *types.RequestContext, _ *types.ToolDef) types.PolicyVerdict {
	path, ok := ctx.Arguments["path"].(string)
	if !ok || path == "" {
		return types.PolicyVerdict{Decision: "ALLOW"}
	}
	expanded := filepath.Clean(path)
	abs, err := filepath.Abs(expanded)
	if err == nil {
		expanded = abs
	}
	for _, prefix := range []string{"/etc/", "/proc/", "/sys/"} {
		if strings.HasPrefix(expanded, prefix) {
			return types.PolicyVerdict{Decision: "DENY", Reason: "access denied: " + path}
		}
	}
	if strings.Contains(expanded, "/.ai/config/secrets") {
		return types.PolicyVerdict{Decision: "DENY", Reason: "access denied: " + path}
	}
	return types.PolicyVerdict{Decision: "ALLOW"}
}

func ruleTimeout(_ *types.RequestContext, _ *types.ToolDef) types.PolicyVerdict {
	return types.PolicyVerdict{Decision: "ALLOW", Reason: "timeout boundary: 30000ms"}
}

func ruleErrorIsolation(ctx *types.RequestContext, _ *types.ToolDef) types.PolicyVerdict {
	if ctx.Status == types.StatusError {
		return types.PolicyVerdict{Decision: "DENY", Reason: "execution error contained — cascade prevented"}
	}
	return types.PolicyVerdict{Decision: "ALLOW"}
}
