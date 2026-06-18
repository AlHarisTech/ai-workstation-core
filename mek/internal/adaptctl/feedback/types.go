// Package feedback implements the ADR-0009 Feedback Controller as a
// pure, stateless rule evaluation engine. It consumes signal history
// and produces adaptive actions deterministically (AC-003).
//
// Boundary: reads signal types only. Never imports MEK internals.
package feedback

import (
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

// ActionType classifies the kind of adaptive action.
type ActionType int

const (
	Notify    ActionType = iota // inform the application
	Reexecute                   // re-run the workflow
	Escalate                    // trigger incident response
	Halt                        // stop dependent operations
)

func (a ActionType) String() string {
	switch a {
	case Notify:
		return "NOTIFY"
	case Reexecute:
		return "REEXECUTE"
	case Escalate:
		return "ESCALATE"
	case Halt:
		return "HALT"
	default:
		return "UNKNOWN"
	}
}

// Action is a recommendation produced by the Feedback Controller.
// It does NOT execute anything — it is a pure data structure (AC-002).
type Action struct {
	ID        string                 `json:"action_id"`
	Type      ActionType             `json:"type"`
	SignalID  string                 `json:"signal_id"`
	RuleID    string                 `json:"rule_id"`
	Target    string                 `json:"target"`
	Payload   map[string]interface{} `json:"payload,omitempty"`
	IssuedAt  time.Time              `json:"issued_at"`
	TTL       time.Duration          `json:"ttl_ms"`
}

// Rule defines a feedback rule that maps signal patterns to actions.
type Rule struct {
	ID             string
	State          signal.State
	MinOccurrences int
	Window         time.Duration   // time window for counting occurrences
	ActionType     ActionType
	Target         string          // application endpoint
	Cooldown       time.Duration   // minimum time between repeated actions
}

// Engine evaluates rules against signal history and produces actions.
// It is stateless — all state is provided via parameters (AC-003).
type Engine struct {
	Rules []Rule
}

// New creates an Engine with the given rules.
func New(rules []Rule) *Engine {
	return &Engine{Rules: rules}
}

// Evaluate runs all rules against the signal history and returns matching actions.
// priorActions contains actions from previous evaluation calls (used for cooldown).
// It is a PURE FUNCTION: same (rules, history, priorActions, now) → same actions.
func (e *Engine) Evaluate(history []signal.Signal, priorActions []Action, now time.Time) []Action {
	var actions []Action

	for _, rule := range e.Rules {
		action := evaluateRule(rule, history, append(priorActions, actions...), now)
		if action != nil {
			actions = append(actions, *action)
		}
	}

	return actions
}

// HasActionType checks if the actions list contains a specific action type.
func HasActionType(actions []Action, t ActionType) bool {
	for _, a := range actions {
		if a.Type == t {
			return true
		}
	}
	return false
}
