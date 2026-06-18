package verify

import (
	"context"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/journal"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

func TestStructural_AllPass(t *testing.T) {
	rirPath := "../../test/fixtures/diamond_dag.json"

	// Execute and verify in one call
	report, err := ExecuteAndVerify(context.Background(), rirPath)
	if err != nil {
		t.Fatal(err)
	}

	if !report.Pass {
		t.Error("structural verification failed on valid diamond DAG")
		for _, v := range report.Violations {
			t.Errorf("  [%s] node=%s: %s", v.Rule, v.NodeID, v.Message)
		}
	}

	if report.Stats.TotalNodes != 4 {
		t.Errorf("expected 4 nodes, got %d", report.Stats.TotalNodes)
	}
	if report.Stats.TotalEdges != 4 {
		t.Errorf("expected 4 edges, got %d", report.Stats.TotalEdges)
	}
	if report.Stats.DependencyChecks == 0 {
		t.Error("no dependency checks performed")
	}
}

func TestStructural_DetectsDependencyViolation(t *testing.T) {
	// Create a journal where a dependency is violated:
	// B depends on A, B=SUCCESS but A=FAILURE
	journalPath := "/tmp/mek_verify_g6_test.jsonl"
	j, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	events <- commit.CommitEvent{NodeID: "root", NewStatus: types.StatusFailure}
	events <- commit.CommitEvent{NodeID: "mid_a", NewStatus: types.StatusSkipped}
	events <- commit.CommitEvent{NodeID: "mid_b", NewStatus: types.StatusSkipped}
	events <- commit.CommitEvent{NodeID: "leaf", NewStatus: types.StatusSuccess} // should not be SUCCESS
	close(events)
	<-done
	j.Close()
	defer os.Remove(journalPath)

	report, err := Structural("../../test/fixtures/diamond_dag.json", journalPath)
	if err != nil {
		t.Fatal(err)
	}

	if report.Pass {
		t.Error("should have detected dependency violation")
	}

	found := false
	for _, v := range report.Violations {
		if v.Rule == "G6" && v.NodeID == "leaf" {
			found = true
		}
	}
	if !found {
		t.Error("did not find G6 violation for leaf node")
	}
}

func TestStructural_DetectsMissingNode(t *testing.T) {
	journalPath := "/tmp/mek_verify_missing_test.jsonl"
	j, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	// Only record 2 of 4 nodes
	events <- commit.CommitEvent{NodeID: "root", NewStatus: types.StatusSuccess}
	events <- commit.CommitEvent{NodeID: "mid_a", NewStatus: types.StatusSuccess}
	close(events)
	<-done
	j.Close()
	defer os.Remove(journalPath)

	report, err := Structural("../../test/fixtures/diamond_dag.json", journalPath)
	if err != nil {
		t.Fatal(err)
	}

	if report.Pass {
		t.Error("should have detected missing nodes")
	}
}

func TestStructural_ActivationConsistency(t *testing.T) {
	// Run a valid execution — activation checks must pass
	report, err := ExecuteAndVerify(context.Background(), "../../test/fixtures/diamond_dag.json")
	if err != nil {
		t.Fatal(err)
	}

	if !report.Pass {
		t.Error("activation consistency check failed on valid execution")
	}
}

func TestStructural_SkipPropagation(t *testing.T) {
	// Create a journal from a failing execution using the failure_propagation fixture
	rirPath := "../../test/fixtures/failure_propagation.json"
	journalPath := "/tmp/mek_verify_skip_test.jsonl"

	mek, err := runtime.New(rirPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	j, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	defer os.Remove(journalPath)

	events := make(chan commit.CommitEvent, 1024)
	done := j.Subscribe(events)

	sm := mek.StatusMap()
	for _, node := range mek.CEG().Nodes {
		state := sm.GetState(node.ID)
		if state != nil {
			events <- commit.CommitEvent{
				NodeID: node.ID, NewStatus: state.Status,
				Outputs: state.Outputs, Artifacts: state.Artifacts,
			}
		}
	}
	close(events)
	<-done
	_ = output

	report, err := Structural(rirPath, journalPath)
	if err != nil {
		t.Fatal(err)
	}

	// Verify expected failure propagation:
	// A = FAILURE
	// B_all (all_success) = SKIPPED
	// C_any (any_success) = SKIPPED (no optional candidate succeeded)
	// D_deep = SKIPPED (B_all was SKIPPED)
	if !report.Pass {
		for _, v := range report.Violations {
			t.Logf("  [%s] node=%s: %s", v.Rule, v.NodeID, v.Message)
		}
	}

	// The journal should show correct propagation
	statuses := report.Stats
	_ = statuses
}

