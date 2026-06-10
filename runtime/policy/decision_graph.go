package policy

import "github.com/AlHarisTech/ai-workstation-core/runtime/types"

type DecisionGraph struct {
	Graph      []types.PolicyDecision `json:"graph"`
	FinalVerdict string               `json:"final_verdict"`
}

func NewDecisionGraph(verdict types.PolicyVerdict) DecisionGraph {
	return DecisionGraph{
		Graph:         verdict.RuleChain,
		FinalVerdict:  verdict.Decision,
	}
}

func (dg *DecisionGraph) Allowed() bool {
	return dg.FinalVerdict == "ALLOW"
}

func (dg *DecisionGraph) DeniedBy() string {
	for _, d := range dg.Graph {
		if d.Decision == "DENY" {
			return d.RuleID
		}
	}
	return ""
}
