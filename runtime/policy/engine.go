package policy

import (
	"sync"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type ToolRegistry interface {
	GetTool(toolID string) (*types.ToolDef, error)
	RequiresSession(toolID string) bool
}

type PolicyEngine struct {
	rules   []PolicyRule
	version string
	mu      sync.RWMutex
}

type PolicyRule struct {
	ID       string
	Name     string
	Stage    string
	Priority int
	Evaluate func(ctx *types.RequestContext, toolDef *types.ToolDef) types.PolicyVerdict
}

func NewPolicyEngine(rules []PolicyRule, version string) *PolicyEngine {
	sorted := make([]PolicyRule, len(rules))
	copy(sorted, rules)
	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Priority > sorted[j].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}
	return &PolicyEngine{rules: sorted, version: version}
}

func (pe *PolicyEngine) EvaluateStage(stage string, ctx *types.RequestContext, toolDef *types.ToolDef) types.PolicyVerdict {
	pe.mu.RLock()
	defer pe.mu.RUnlock()

	var chain []types.PolicyDecision
	for _, rule := range pe.rules {
		if rule.Stage != stage {
			continue
		}
		verdict := rule.Evaluate(ctx, toolDef)
		chain = append(chain, types.PolicyDecision{
			RuleID: rule.ID, Decision: verdict.Decision,
			Reason: verdict.Reason, Stage: stage, Priority: rule.Priority,
		})
		ctx.PolicyGraph = append(ctx.PolicyGraph, chain[len(chain)-1])
		if verdict.Decision == "DENY" {
			return types.PolicyVerdict{
				Decision:  "DENY",
				Reason:    verdict.Reason,
				RuleID:    rule.ID,
				RuleChain: chain,
				Timestamp: verdict.Timestamp,
			}
		}
	}
	return types.PolicyVerdict{
		Decision:  "ALLOW",
		RuleID:    "default-allow",
		RuleChain: chain,
		Reason:    "no policies deny at stage " + stage,
	}
}

func (pe *PolicyEngine) Version() string { return pe.version }
func (pe *PolicyEngine) RuleCount() int  { return len(pe.rules) }
