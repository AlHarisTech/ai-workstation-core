package mcpv2

import "sync"

type PolicyRule struct {
	Allowed bool
	Reason  string
}

type EnforcementEngine struct {
	mu       sync.RWMutex
	policies map[string]PolicyRule
}

type EnforcementResult struct {
	Allowed     bool
	BlockReason string
	Server      string
	Operation   string
}

func NewEnforcementEngine() *EnforcementEngine {
	return &EnforcementEngine{
		policies: make(map[string]PolicyRule),
	}
}

func policyKey(server, operation string) string {
	return server + ":" + operation
}

func (ee *EnforcementEngine) SetRule(server, operation string, allowed bool, reason string) {
	ee.mu.Lock()
	defer ee.mu.Unlock()
	ee.policies[policyKey(server, operation)] = PolicyRule{Allowed: allowed, Reason: reason}
}

func (ee *EnforcementEngine) Check(server, operation string) EnforcementResult {
	ee.mu.RLock()
	defer ee.mu.RUnlock()
	rule, ok := ee.policies[policyKey(server, operation)]
	if !ok {
		return EnforcementResult{Allowed: true, Server: server, Operation: operation}
	}
	if !rule.Allowed {
		return EnforcementResult{
			Allowed:     false,
			BlockReason: rule.Reason,
			Server:      server,
			Operation:   operation,
		}
	}
	return EnforcementResult{Allowed: true, Server: server, Operation: operation}
}
