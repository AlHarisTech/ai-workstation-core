package mcpv2

import (
	"fmt"
	"strings"
)

type PolicyEngine struct{}

func NewPolicyEngine() *PolicyEngine {
	return &PolicyEngine{}
}

func (pe *PolicyEngine) Enforce(action ActionType, operation string, policy MCPPolicy) error {
	actionStr := string(action) + "." + operation

	// Rule: deny matched → REJECT
	for _, d := range policy.Deny {
		if matchPolicy(d, actionStr) {
			return fmt.Errorf("policy deny: %s matches %s", actionStr, d)
		}
	}

	// Rule: if allow is non-empty, must have a match
	if len(policy.Allow) > 0 {
		allowed := false
		for _, a := range policy.Allow {
			if matchPolicy(a, actionStr) {
				allowed = true
				break
			}
		}
		if !allowed {
			return fmt.Errorf("policy deny (default): %s not in allow list", actionStr)
		}
	}

	return nil
}

func matchPolicy(pattern, action string) bool {
	if pattern == "*" || pattern == "*.*" {
		return true
	}
	parts := strings.SplitN(pattern, ".", 2)
	actionParts := strings.SplitN(action, ".", 2)
	if len(parts) == 0 || len(actionParts) == 0 {
		return false
	}
	if parts[0] != "*" && parts[0] != actionParts[0] {
		return false
	}
	if len(parts) == 1 {
		return len(actionParts) >= 1
	}
	if parts[1] == "*" {
		return len(actionParts) >= 2
	}
	return len(actionParts) >= 2 && parts[1] == actionParts[1]
}
