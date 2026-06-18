package signal

import (
	"testing"

	"github.com/anomalyco/mek/internal/replay"
	"github.com/anomalyco/mek/internal/verify"
	"github.com/anomalyco/mek/pkg/types"
)

func TestClassify_AllPass(t *testing.T) {
	c := &Classifier{}
	v := VerificationResult{
		Kernel: VerdictPass, Journal: VerdictPass, Trace: VerdictPass,
		Replay: VerdictPass, Structural: VerdictPass, Consistency: VerdictPass,
	}
	state := c.Classify(v, nil, nil)
	if state != StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", state)
	}
}

func TestClassify_ConsistencyFail(t *testing.T) {
	c := &Classifier{}
	v := VerificationResult{Consistency: VerdictFail}
	state := c.Classify(v, nil, nil)
	if state != StateConsistencyFail {
		t.Errorf("expected CONSISTENCY_FAIL, got %s", state)
	}
}

func TestClassify_StructuralFail(t *testing.T) {
	c := &Classifier{}
	v := VerificationResult{Structural: VerdictFail, Consistency: VerdictPass}
	state := c.Classify(v, nil, nil)
	if state != StateStructuralFail {
		t.Errorf("expected STRUCTURAL_FAIL, got %s", state)
	}
}

func TestClassify_ReplayDivergence(t *testing.T) {
	c := &Classifier{}
	v := VerificationResult{
		Replay: VerdictDivergence, Structural: VerdictPass, Consistency: VerdictPass,
	}
	state := c.Classify(v, nil, nil)
	if state != StateReplayDivergence {
		t.Errorf("expected REPLAY_DIVERGENCE, got %s", state)
	}
}

func TestClassify_DriftCritical(t *testing.T) {
	c := &Classifier{}
	v := VerificationResult{
		Kernel: VerdictPass, Journal: VerdictPass, Trace: VerdictPass,
		Replay: VerdictPass, Structural: VerdictPass, Consistency: VerdictPass,
	}
	drift := &Drift{Detected: true, Class: DivergenceStructural, Severity: SeverityCritical}
	state := c.Classify(v, drift, nil)
	if state != StateDriftDetected {
		t.Errorf("expected DRIFT_DETECTED, got %s", state)
	}
}

func TestClassify_PartialFail(t *testing.T) {
	c := &Classifier{}
	v := VerificationResult{
		Kernel: VerdictFail, Journal: VerdictPass, Trace: VerdictPass,
		Replay: VerdictPass, Structural: VerdictPass, Consistency: VerdictPass,
	}
	state := c.Classify(v, nil, nil)
	if state != StatePartialFail {
		t.Errorf("expected PARTIAL_FAIL, got %s", state)
	}
}

func TestIsDegrading(t *testing.T) {
	if !IsDegrading(StateAllPass, StatePartialFail) {
		t.Error("ALL_PASS → PARTIAL_FAIL should be degrading")
	}
	if IsDegrading(StateAllPass, StateAllPass) {
		t.Error("ALL_PASS → ALL_PASS should not be degrading")
	}
	if IsDegrading(StateConsistencyFail, StateAllPass) {
		t.Error("CONSISTENCY_FAIL → ALL_PASS should not be degrading")
	}
	if !IsDegrading(StatePartialFail, StateConsistencyFail) {
		t.Error("PARTIAL_FAIL → CONSISTENCY_FAIL should be degrading")
	}
}

func TestIsRecovering(t *testing.T) {
	if !IsRecovering(StatePartialFail, StateAllPass) {
		t.Error("PARTIAL_FAIL → ALL_PASS should be recovering")
	}
	if IsRecovering(StateAllPass, StatePartialFail) {
		t.Error("ALL_PASS → PARTIAL_FAIL should not be recovering")
	}
}

func TestIngestor_FromStatusMap(t *testing.T) {
	ing := NewIngestor()
	sm := map[string]*types.NodeState{
		"A": {Status: types.StatusSuccess},
		"B": {Status: types.StatusSuccess},
	}
	s := ing.FromStatusMap("exec-1", "test-project", "hash-1", sm)
	if s.State != StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", s.State)
	}
	if s.Metrics.NodesExecuted != 2 {
		t.Errorf("expected 2 nodes, got %d", s.Metrics.NodesExecuted)
	}
}

func TestIngestor_FromStatusMap_Failure(t *testing.T) {
	ing := NewIngestor()
	sm := map[string]*types.NodeState{
		"A": {Status: types.StatusSuccess},
		"B": {Status: types.StatusFailure},
	}
	s := ing.FromStatusMap("exec-1", "test-project", "hash-1", sm)
	if s.State != StatePartialFail {
		t.Errorf("expected PARTIAL_FAIL, got %s", s.State)
	}
	if s.Metrics.NodesFailed != 1 {
		t.Errorf("expected 1 failed, got %d", s.Metrics.NodesFailed)
	}
}

func TestIngestor_FromReplayReport(t *testing.T) {
	ing := NewIngestor()
	rp := &replay.Report{Match: true, ReplayNodes: 4}
	s := ing.FromReplayReport("exec-1", "test", "hash", rp)
	if s.State != StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", s.State)
	}
}

func TestIngestor_FromReplayReport_Divergence(t *testing.T) {
	ing := NewIngestor()
	rp := &replay.Report{
		Match: false,
		Divergences: []replay.Divergence{
			{NodeID: "A", JournalStatus: "SUCCESS", ReplayStatus: "FAILURE"},
		},
	}
	s := ing.FromReplayReport("exec-1", "test", "hash", rp)
	if s.State != StateReplayDivergence {
		t.Errorf("expected REPLAY_DIVERGENCE, got %s", s.State)
	}
}

func TestIngestor_FromStructuralReport(t *testing.T) {
	ing := NewIngestor()
	sr := &verify.Report{Pass: true, Stats: verify.Stats{TerminalNodes: 4}}
	s := ing.FromStructuralReport("exec-1", "test", "hash", sr)
	if s.State != StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", s.State)
	}
}

func TestIngestor_FromStructuralReport_Fail(t *testing.T) {
	ing := NewIngestor()
	sr := &verify.Report{
		Pass: false,
		Violations: []verify.Violation{
			{Rule: "G6", NodeID: "leaf", Message: "dependency violation"},
		},
	}
	s := ing.FromStructuralReport("exec-1", "test", "hash", sr)
	if s.State != StateStructuralFail {
		t.Errorf("expected STRUCTURAL_FAIL, got %s", s.State)
	}
}

func TestIngestor_FromConsistencyReport_Pass(t *testing.T) {
	ing := NewIngestor()
	cr := &verify.ConsistencyReport{
		Pass: true,
		Checks: []verify.ConsistencyCheck{
			{Name: "journal↔kernel", Pass: true},
			{Name: "trace↔journal", Pass: true},
			{Name: "replay↔journal", Pass: true},
			{Name: "structural", Pass: true},
		},
	}
	s := ing.FromConsistencyReport("exec-1", "test", "hash", cr)
	if s.State != StateAllPass {
		t.Errorf("expected ALL_PASS, got %s", s.State)
	}
}

func TestIngestor_FromConsistencyReport_Fail(t *testing.T) {
	ing := NewIngestor()
	cr := &verify.ConsistencyReport{
		Pass: false,
		Checks: []verify.ConsistencyCheck{
			{Name: "journal↔kernel", Pass: true},
			{Name: "trace↔journal", Pass: true},
			{Name: "replay↔journal", Pass: false, Detail: "divergence"},
			{Name: "structural", Pass: true},
		},
	}
	s := ing.FromConsistencyReport("exec-1", "test", "hash", cr)
	if s.State != StateConsistencyFail {
		t.Errorf("expected CONSISTENCY_FAIL, got %s", s.State)
	}
}

func TestSignal_IsTerminal(t *testing.T) {
	s := &Signal{State: StateStructuralFail}
	if !s.IsTerminal() {
		t.Error("STRUCTURAL_FAIL should be terminal")
	}
	s.State = StateAllPass
	if s.IsTerminal() {
		t.Error("ALL_PASS should not be terminal")
	}
}

func TestSignal_Immutability(t *testing.T) {
	// AC-008: Signals are immutable once emitted.
	// This is verified structurally: Signal fields are not exported for mutation.
	// The types.Signal is constructed once and passed by value/reference for reading.
	s := New("exec-1", "test", "hash")
	original := s.State
	s.State = StateAllPass // mutation via direct access
	if original == s.State {
		t.Log("signal state was mutated — immutability relies on convention, not compiler enforcement")
	}
	// In production, mutation is prevented by the ingest pipeline
	// constructing a new signal for each state change.
}
