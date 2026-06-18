package feedback

import (
	"fmt"
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

// evaluateRule checks if a single rule should fire given the signal history.
// Returns nil if the rule conditions are not met.
func evaluateRule(rule Rule, history []signal.Signal, priorActions []Action, now time.Time) *Action {
	// 1. Filter signals matching the rule's state within the time window
	matches := filterByState(history, rule.State, rule.Window, now)

	// 2. Check minimum occurrences
	if len(matches) < rule.MinOccurrences {
		return nil
	}

	// 3. Check cooldown: has this rule fired recently?
	if isInCooldown(rule, priorActions, now) {
		return nil
	}

	// 4. Take the most recent matching signal as the trigger
	trigger := matches[len(matches)-1]

	return &Action{
		ID:       fmt.Sprintf("%s-%s-%d", rule.ID, trigger.ID, now.UnixMilli()),
		Type:     rule.ActionType,
		SignalID: trigger.ID,
		RuleID:   rule.ID,
		Target:   rule.Target,
		Payload: map[string]interface{}{
			"signal_state":     string(trigger.State),
			"occurrences":      len(matches),
			"window":           rule.Window.String(),
			"trigger_timestamp": trigger.Timestamp.Format(time.RFC3339),
		},
		IssuedAt: now,
		TTL:      rule.Cooldown,
	}
}

// filterByState returns signals matching the given state within the time window.
func filterByState(history []signal.Signal, state signal.State, window time.Duration, now time.Time) []signal.Signal {
	var matches []signal.Signal
	cutoff := now.Add(-window)

	for _, s := range history {
		if s.State == state && s.Timestamp.After(cutoff) {
			matches = append(matches, s)
		}
	}

	// Ensure consecutive occurrences
	if len(matches) > 1 {
		consecutive := []signal.Signal{matches[len(matches)-1]}
		for i := len(matches) - 2; i >= 0; i-- {
			if matches[i].State == state {
				consecutive = append([]signal.Signal{matches[i]}, consecutive...)
			} else {
				break
			}
		}
		return consecutive
	}

	return matches
}

// isInCooldown checks if the rule has produced an action within its cooldown period.
func isInCooldown(rule Rule, actions []Action, now time.Time) bool {
	if rule.Cooldown == 0 {
		return false
	}
	cutoff := now.Add(-rule.Cooldown)
	for _, a := range actions {
		if a.RuleID == rule.ID && a.IssuedAt.After(cutoff) {
			return true
		}
	}
	return false
}
