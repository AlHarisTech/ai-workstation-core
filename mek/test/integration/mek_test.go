package integration

import (
	"context"
	"testing"

	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

func TestSimpleDAG(t *testing.T) {
	mek, err := runtime.New("../../test/fixtures/simple_dag.json", nil)
	if err != nil {
		t.Fatalf("failed to create MEK: %v", err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatalf("MEK execution failed: %v", err)
	}

	// Verify all nodes reached terminal state
	for _, nodeID := range []string{"A", "B", "C"} {
		state := output.StatusMap[nodeID]
		if state == nil {
			t.Errorf("node %s not found in status map", nodeID)
			continue
		}
		if !state.Status.IsTerminal() {
			t.Errorf("node %s not terminal: %s", nodeID, state.Status)
		}
	}

	// M-001: Verify SUCCESS nodes
	if output.StatusMap["A"].Status != types.StatusSuccess {
		t.Errorf("root node A expected SUCCESS, got %s", output.StatusMap["A"].Status)
	}
	if output.StatusMap["C"].Status != types.StatusSuccess {
		t.Errorf("root node C expected SUCCESS, got %s", output.StatusMap["C"].Status)
	}

	// Verify metrics
	if output.Metrics.NodesExecuted != 3 {
		t.Errorf("expected 3 nodes executed, got %d", output.Metrics.NodesExecuted)
	}
	if output.Metrics.NodesFailed != 0 {
		t.Errorf("expected 0 failed nodes, got %d", output.Metrics.NodesFailed)
	}
	if output.Metrics.EscalationRequested {
		t.Error("escalation should not have been requested")
	}
}

func TestESCALATEGate(t *testing.T) {
	mek, err := runtime.New("../../test/fixtures/escalate_dag.json", nil)
	if err != nil {
		t.Fatalf("failed to create MEK: %v", err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatalf("MEK execution failed: %v", err)
	}

	// Verify escalation was triggered
	if !output.Metrics.EscalationRequested {
		t.Error("ESCALATE should have been triggered")
	}

	// Gate should be TERMINATED (it triggered escalation)
	gateState := output.StatusMap["decision_gate"]
	if gateState == nil {
		t.Fatal("gate node not found")
	}
	if gateState.Status != types.StatusTerminated {
		t.Errorf("gate should be TERMINATED, got %s", gateState.Status)
	}

	// Normal path should be TERMINATED (escalation terminated all remaining)
	normalState := output.StatusMap["normal_path"]
	if normalState == nil {
		t.Fatal("normal_path node not found")
	}
	if normalState.Status != types.StatusTerminated {
		t.Errorf("normal_path should be TERMINATED after escalation, got %s", normalState.Status)
	}
}

func TestDeterminism(t *testing.T) {
	// Run the same DAG twice — should produce identical results
	run1, err := runtime.New("../../test/fixtures/simple_dag.json", nil)
	if err != nil {
		t.Fatalf("run1: %v", err)
	}
	out1, err := run1.Run(context.Background())
	if err != nil {
		t.Fatalf("run1 failed: %v", err)
	}

	run2, err := runtime.New("../../test/fixtures/simple_dag.json", nil)
	if err != nil {
		t.Fatalf("run2: %v", err)
	}
	out2, err := run2.Run(context.Background())
	if err != nil {
		t.Fatalf("run2 failed: %v", err)
	}

	// G1: Same RIR → same results
	for _, nodeID := range []string{"A", "B", "C"} {
		s1 := out1.StatusMap[nodeID]
		s2 := out2.StatusMap[nodeID]
		if s1 == nil || s2 == nil {
			t.Errorf("node %s missing in one run", nodeID)
			continue
		}
		if s1.Status != s2.Status {
			t.Errorf("node %s: run1=%s run2=%s (determinism violated)", nodeID, s1.Status, s2.Status)
		}
	}
}

func TestMEKMetrics(t *testing.T) {
	mek, err := runtime.New("../../test/fixtures/simple_dag.json", nil)
	if err != nil {
		t.Fatalf("failed to create MEK: %v", err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatalf("MEK execution failed: %v", err)
	}

	// G4: Bounded termination — duration must be set (≥ 0)
	if output.Metrics.TotalDurationMs < 0 {
		t.Error("total duration not set")
	}

	// Wave count should be positive
	if output.Metrics.WavesCompleted <= 0 {
		t.Error("no waves completed")
	}
}
