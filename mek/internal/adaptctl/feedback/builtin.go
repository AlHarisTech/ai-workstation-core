package feedback

import (
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

// BuiltinRules returns the six standard ADR-0009 feedback rules (R01–R06).
func BuiltinRules() []Rule {
	return []Rule{
		{
			ID:             "R01",
			State:          signal.StateAllPass,
			MinOccurrences: 3,
			Window:         5 * time.Minute,
			ActionType:     Notify,
			Target:         "app://status",
			Cooldown:       10 * time.Minute,
		},
		{
			ID:             "R02",
			State:          signal.StateReplayDivergence,
			MinOccurrences: 1,
			Window:         time.Minute,
			ActionType:     Reexecute,
			Target:         "app://workflow",
			Cooldown:       30 * time.Second,
		},
		{
			ID:             "R03",
			State:          signal.StateStructuralFail,
			MinOccurrences: 1,
			Window:         time.Minute,
			ActionType:     Halt,
			Target:         "app://dependent-ops",
			Cooldown:       0, // no cooldown — halt immediately
		},
		{
			ID:             "R04",
			State:          signal.StateDriftDetected,
			MinOccurrences: 1,
			Window:         time.Minute,
			ActionType:     Escalate,
			Target:         "app://incident",
			Cooldown:       5 * time.Minute,
		},
		{
			ID:             "R05",
			State:          signal.StateDriftDetected,
			MinOccurrences: 5,
			Window:         time.Hour,
			ActionType:     Notify,
			Target:         "app://status",
			Cooldown:       30 * time.Minute,
		},
		{
			ID:             "R06",
			State:          signal.StateConsistencyFail,
			MinOccurrences: 1,
			Window:         time.Minute,
			ActionType:     Halt,
			Target:         "app://all-workflows",
			Cooldown:       0, // no cooldown — halt immediately
		},
	}
}
