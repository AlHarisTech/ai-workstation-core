package predict

import (
	"testing"
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

func now() time.Time {
	return time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
}

func makeSignal(state signal.State, offset time.Duration) signal.Signal {
	return signal.Signal{
		ID:        "sig-" + string(state),
		Timestamp: now().Add(offset),
		State:     state,
	}
}

func makeHistory(states ...signal.State) []signal.Signal {
	var h []signal.Signal
	for i, s := range states {
		h = append(h, signal.Signal{
			ID:        "sig-" + string(s),
			Timestamp: now().Add(-time.Duration(len(states)-i) * time.Minute),
			State:     s,
		})
	}
	return h
}

// ─── Empty / Minimal History ───

func TestEmptyHistory(t *testing.T) {
	p := Predict(nil, time.Hour, now())
	if p.State != signal.StateAllPass {
		t.Errorf("empty history: expected ALL_PASS, got %s", p.State)
	}
	if p.Confidence > 0.1 {
		t.Errorf("empty history: confidence should be low, got %.2f", p.Confidence)
	}
}

func TestSingleSignal(t *testing.T) {
	history := []signal.Signal{
		makeSignal(signal.StateAllPass, -time.Minute),
	}
	p := Predict(history, time.Hour, now())

	if p.State != signal.StateAllPass {
		t.Errorf("single ALL_PASS: expected ALL_PASS, got %s", p.State)
	}
	if p.Probability < 0.5 {
		t.Errorf("single ALL_PASS: probability too low: %.2f", p.Probability)
	}
	if p.SampleSize != 1 {
		t.Errorf("expected sample size 1, got %d", p.SampleSize)
	}
}

// ─── Structural Fail ───

func TestStructuralFail_Deterministic(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StateStructuralFail,
	)
	p := Predict(history, time.Hour, now())

	if p.State != signal.StateStructuralFail {
		t.Errorf("expected STRUCTURAL_FAIL, got %s", p.State)
	}
	if p.Probability != 1.0 {
		t.Errorf("structural fail probability should be 1.0, got %.2f", p.Probability)
	}
	if p.Direction != Degrading {
		t.Errorf("structural fail direction should be DEGRADING, got %s", p.Direction)
	}
	if p.Confidence != 1.0 {
		t.Errorf("structural fail confidence should be 1.0, got %.2f", p.Confidence)
	}
}

// ─── Replay Divergence ───

func TestReplayDivergence_Forecast(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateReplayDivergence,
		signal.StateAllPass,
	)
	p := Predict(history, 2*time.Hour, now())

	if p.State != signal.StateReplayDivergence {
		t.Errorf("expected REPLAY_DIVERGENCE, got %s", p.State)
	}
	if p.Probability <= 0 {
		t.Errorf("replay divergence probability should be > 0, got %.2f", p.Probability)
	}
	if len(p.Factors) == 0 {
		t.Error("should have replay_divergence_rate factor")
	}
}

// ─── Drift Detected ───

func TestDriftDetected_Degrading(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateDriftDetected,
		signal.StateDriftDetected,
		signal.StateDriftDetected,
		signal.StateDriftDetected,
	)
	p := Predict(history, time.Hour, now())

	if p.State != signal.StateDriftDetected {
		t.Errorf("expected DRIFT_DETECTED, got %s", p.State)
	}
}

// ─── All Pass ───

func TestAllPass_Stable(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StateAllPass,
	)
	p := Predict(history, time.Hour, now())

	if p.State != signal.StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", p.State)
	}
	if p.Direction != Stable {
		t.Errorf("expected STABLE, got %s", p.Direction)
	}
	if p.Probability < 0.8 {
		t.Errorf("all-pass probability should be high, got %.2f", p.Probability)
	}
}

// ─── Direction ───

func TestDirection_Degrading(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StatePartialFail,
		signal.StatePartialFail,
		signal.StateDriftDetected,
	)
	p := Predict(history, time.Hour, now())

	if p.Direction != Degrading {
		t.Errorf("expected DEGRADING, got %s", p.Direction)
	}
}

func TestDirection_Improving(t *testing.T) {
	// Recent signals are better than older ones
	now := now()
	history := []signal.Signal{
		{ID: "s1", Timestamp: now.Add(-50 * time.Minute), State: signal.StatePartialFail},
		{ID: "s2", Timestamp: now.Add(-40 * time.Minute), State: signal.StateDriftDetected},
		{ID: "s3", Timestamp: now.Add(-30 * time.Minute), State: signal.StateAllPass},
		{ID: "s4", Timestamp: now.Add(-20 * time.Minute), State: signal.StateAllPass},
		{ID: "s5", Timestamp: now.Add(-10 * time.Minute), State: signal.StateAllPass},
		{ID: "s6", Timestamp: now.Add(-1 * time.Minute), State: signal.StateAllPass},
	}
	p := Predict(history, time.Hour, now)

	if p.State != signal.StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", p.State)
	}
	if p.Direction != Improving && p.Direction != Stable {
		t.Errorf("expected IMPROVING or STABLE, got %s", p.Direction)
	}
}

// ─── Confidence ───

func TestConfidence_LowSample(t *testing.T) {
	p := Predict(
		[]signal.Signal{makeSignal(signal.StateAllPass, -time.Minute)},
		time.Hour, now(),
	)
	if p.Confidence > 0.2 {
		t.Errorf("low sample should have low confidence, got %.2f", p.Confidence)
	}
}

func TestConfidence_HighSample(t *testing.T) {
	var history []signal.Signal
	for i := 0; i < 100; i++ {
		history = append(history, makeSignal(signal.StateAllPass, -time.Duration(100-i)*time.Minute))
	}
	p := Predict(history, time.Hour, now())

	if p.Confidence < 0.9 {
		t.Errorf("high sample should have high confidence, got %.2f", p.Confidence)
	}
}

// ─── Consistency Fail ───

func TestConsistencyFail_Deterministic(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateConsistencyFail,
	)
	p := Predict(history, time.Hour, now())

	if p.State != signal.StateConsistencyFail {
		t.Errorf("expected CONSISTENCY_FAIL, got %s", p.State)
	}
	if p.Probability != 1.0 {
		t.Errorf("consistency fail probability should be 1.0, got %.2f", p.Probability)
	}
}

// ─── Horizon Effects ───

func TestHorizon_IncreasesProbability(t *testing.T) {
	// Use a divergence from further in the past (lower rate) so that
	// probability doesn't immediately saturate at 1.0
	history := []signal.Signal{
		makeSignal(signal.StateReplayDivergence, -30*time.Minute),
	}
	pShort := Predict(history, time.Minute, now())
	pLong := Predict(history, 2*time.Hour, now())

	if pLong.Probability <= pShort.Probability {
		t.Errorf("longer horizon should increase probability: short=%.2f long=%.2f",
			pShort.Probability, pLong.Probability)
	}
}

// ─── Mixed Signals ───

func TestMixedSignals_Priority(t *testing.T) {
	// Structural fail should dominate over drift
	history := makeHistory(
		signal.StateDriftDetected,
		signal.StateDriftDetected,
		signal.StateStructuralFail,
		signal.StateDriftDetected,
	)
	p := Predict(history, time.Hour, now())

	if p.State != signal.StateStructuralFail {
		t.Errorf("STRUCTURAL_FAIL should dominate, got %s", p.State)
	}
}

// ─── Determinism ───

func TestDeterminism(t *testing.T) {
	history := makeHistory(
		signal.StateAllPass,
		signal.StateAllPass,
		signal.StatePartialFail,
		signal.StateReplayDivergence,
		signal.StateAllPass,
	)

	var first Prediction
	for i := 0; i < 500; i++ {
		p := Predict(history, time.Hour, now())
		if i == 0 {
			first = p
			continue
		}
		if p.State != first.State {
			t.Fatalf("run %d: state %s ≠ %s", i, p.State, first.State)
		}
		if p.Probability != first.Probability {
			t.Fatalf("run %d: probability %.2f ≠ %.2f", i, p.Probability, first.Probability)
		}
		if p.Direction != first.Direction {
			t.Fatalf("run %d: direction %s ≠ %s", i, p.Direction, first.Direction)
		}
	}
}

// ─── Direction String ───

func TestDirectionString(t *testing.T) {
	if Stable.String() != "STABLE" {
		t.Errorf("Stable: %s", Stable.String())
	}
	if Degrading.String() != "DEGRADING" {
		t.Errorf("Degrading: %s", Degrading.String())
	}
	if Improving.String() != "IMPROVING" {
		t.Errorf("Improving: %s", Improving.String())
	}
}
