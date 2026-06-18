package feedback

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

func now() time.Time {
	return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
}

func makeSignal(state signal.State, offset time.Duration) signal.Signal {
	return signal.Signal{
		ID:        fmt.Sprintf("sig-%d", offset.Milliseconds()),
		Timestamp: now().Add(offset),
		State:     state,
	}
}

func makeSignals(state signal.State, count int, interval time.Duration) []signal.Signal {
	var sigs []signal.Signal
	for i := 0; i < count; i++ {
		sigs = append(sigs, makeSignal(state, interval*time.Duration(i)))
	}
	return sigs
}

// ─── R01: ALL_PASS ×3 → NOTIFY ───

func TestR01_AllPassThreeTimes(t *testing.T) {
	engine := New(BuiltinRules())
	history := makeSignals(signal.StateAllPass, 3, time.Second)
	actions := engine.Evaluate(history, nil, now())

	if !HasActionType(actions, Notify) {
		t.Error("R01: expected NOTIFY for ALL_PASS ×3")
	}
}

func TestR01_InsufficientOccurrences(t *testing.T) {
	engine := New(BuiltinRules())
	history := makeSignals(signal.StateAllPass, 2, time.Second)
	actions := engine.Evaluate(history, nil, now())

	if HasActionType(actions, Notify) {
		t.Error("R01: should not fire with only 2 occurrences")
	}
}

// ─── R02: REPLAY_DIVERGENCE → REEXECUTE ───

func TestR02_ReplayDivergence(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateReplayDivergence, 0),
	}
	actions := engine.Evaluate(history, nil, now())

	if !HasActionType(actions, Reexecute) {
		t.Error("R02: expected REEXECUTE for REPLAY_DIVERGENCE")
	}
}

// ─── R03: STRUCTURAL_FAIL → HALT ───

func TestR03_StructuralFail(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateStructuralFail, 0),
	}
	actions := engine.Evaluate(history, nil, now())

	if !HasActionType(actions, Halt) {
		t.Error("R03: expected HALT for STRUCTURAL_FAIL")
	}
}

// ─── R04: DRIFT_DETECTED CRITICAL → ESCALATE ───

func TestR04_DriftCritical(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateDriftDetected, 0),
	}
	actions := engine.Evaluate(history, nil, now())

	if !HasActionType(actions, Escalate) {
		t.Error("R04: expected ESCALATE for DRIFT_DETECTED")
	}
}

// ─── R05: DRIFT_DETECTED ×5 in 1h → NOTIFY ───

func TestR05_DriftLowMultiple(t *testing.T) {
	engine := New(BuiltinRules())
	// 5 drift signals within 1 hour
	history := makeSignals(signal.StateDriftDetected, 5, 10*time.Minute)
	actions := engine.Evaluate(history, nil, now())

	// Should have both R04 (single drift → escalate) and R05 (5 drifts → notify)
	if !HasActionType(actions, Escalate) {
		t.Error("R04: expected ESCALATE for DRIFT_DETECTED")
	}
	if !HasActionType(actions, Notify) {
		t.Error("R05: expected NOTIFY for DRIFT_DETECTED ×5")
	}
}

func TestR05_InsufficientDrift(t *testing.T) {
	engine := New(BuiltinRules())
	history := makeSignals(signal.StateDriftDetected, 3, 10*time.Minute)
	actions := engine.Evaluate(history, nil, now())

	// R04 fires (single drift) but R05 does not (need 5)
	if !HasActionType(actions, Escalate) {
		t.Error("R04: expected ESCALATE for DRIFT_DETECTED")
	}
	// Count NOTIFY — only R01 could produce NOTIFY, not R05
	notifyCount := 0
	for _, a := range actions {
		if a.Type == Notify && a.RuleID == "R05" {
			notifyCount++
		}
	}
	if notifyCount > 0 {
		t.Error("R05: should not fire with only 3 occurrences")
	}
}

// ─── R06: CONSISTENCY_FAIL → HALT ───

func TestR06_ConsistencyFail(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateConsistencyFail, 0),
	}
	actions := engine.Evaluate(history, nil, now())

	if !HasActionType(actions, Halt) {
		t.Error("R06: expected HALT for CONSISTENCY_FAIL")
	}
}

// ─── Cooldown ───

func TestCooldown_PreventsReFire(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateReplayDivergence, -10*time.Second),
		makeSignal(signal.StateReplayDivergence, 0),
	}

	// First evaluation fires R02
	actions1 := engine.Evaluate(history, nil, now())
	if !HasActionType(actions1, Reexecute) {
		t.Fatal("R02 should fire initially")
	}

	// Second evaluation with prior actions — cooldown should prevent re-fire
	actions2 := engine.Evaluate(history, actions1, now())
	// R02 has 30s cooldown — should not fire again
	for _, a := range actions2 {
		if a.RuleID == "R02" {
			t.Errorf("R02 should be in cooldown, but fired again at %v", a.IssuedAt)
		}
	}
}

// ─── Edge Cases ───

func TestEmptyHistory(t *testing.T) {
	engine := New(BuiltinRules())
	actions := engine.Evaluate(nil, nil, now())
	if len(actions) != 0 {
		t.Errorf("empty history should produce no actions, got %d", len(actions))
	}
}

func TestOutOfOrderSignals(t *testing.T) {
	engine := New(BuiltinRules())
	// Signals arrive in reverse chronological order
	history := []signal.Signal{
		makeSignal(signal.StateAllPass, 2*time.Second),
		makeSignal(signal.StateAllPass, 1*time.Second),
		makeSignal(signal.StateAllPass, 0),
	}
	actions := engine.Evaluate(history, nil, now())
	// Still 3 consecutive ALL_PASS — should fire
	if !HasActionType(actions, Notify) {
		t.Error("R01 should fire even with out-of-order signals")
	}
}

func TestMixedStates(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateAllPass, 0),
		makeSignal(signal.StatePartialFail, time.Second),
		makeSignal(signal.StateAllPass, 2*time.Second),
	}
	actions := engine.Evaluate(history, nil, now())
	// ALL_PASS not consecutive — R01 should not fire
	if HasActionType(actions, Notify) {
		t.Error("R01 should not fire with non-consecutive ALL_PASS")
	}
}

// ─── Determinism ───

func TestDeterminism(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateAllPass, 0),
		makeSignal(signal.StateAllPass, time.Second),
		makeSignal(signal.StateAllPass, 2*time.Second),
		makeSignal(signal.StateReplayDivergence, 3*time.Second),
	}

	var first []Action
	for i := 0; i < 500; i++ {
		actions := engine.Evaluate(history, nil, now())
		if first == nil {
			first = actions
			continue
		}
		if len(actions) != len(first) {
			t.Fatalf("run %d: action count %d ≠ %d", i, len(actions), len(first))
		}
		for j := range actions {
			if actions[j].Type != first[j].Type || actions[j].RuleID != first[j].RuleID {
				t.Fatalf("run %d action %d: %s/%s ≠ %s/%s",
					i, j, actions[j].Type, actions[j].RuleID,
					first[j].Type, first[j].RuleID)
			}
		}
	}
}

// ─── Action Sorting ───

func TestActionsSorted(t *testing.T) {
	engine := New(BuiltinRules())
	history := []signal.Signal{
		makeSignal(signal.StateDriftDetected, 0),
		makeSignal(signal.StateDriftDetected, 10*time.Minute),
		makeSignal(signal.StateDriftDetected, 20*time.Minute),
		makeSignal(signal.StateDriftDetected, 30*time.Minute),
		makeSignal(signal.StateDriftDetected, 40*time.Minute),
	}
	actions := engine.Evaluate(history, nil, now())

	// Actions should be in rule evaluation order (as they appear in rules list)
	if !sort.SliceIsSorted(actions, func(i, j int) bool {
		return actions[i].IssuedAt.Before(actions[j].IssuedAt) ||
			(actions[i].IssuedAt.Equal(actions[j].IssuedAt) && actions[i].RuleID < actions[j].RuleID)
	}) {
		t.Error("actions should maintain deterministic ordering")
	}
}
