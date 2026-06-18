package replay

import (
	"context"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/journal"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

func TestReplay_DeterministicMatch(t *testing.T) {
	// Run MEK once and record journal
	journalPath := "/tmp/mek_replay_test.jsonl"
	rirPath := "../../test/fixtures/diamond_dag.json"

	recordJournal(t, rirPath, journalPath)
	defer os.Remove(journalPath)

	// Replay and verify
	report, err := Verify(context.Background(), rirPath, journalPath)
	if err != nil {
		t.Fatal(err)
	}

	if !report.Match {
		t.Errorf("REPLAY DIVERGENCE: %d divergences", len(report.Divergences))
		for _, d := range report.Divergences {
			t.Errorf("  node %s: journal=%s replay=%s", d.NodeID, d.JournalStatus, d.ReplayStatus)
		}
	}

	if report.JournalEntries != 4 {
		t.Errorf("expected 4 journal entries, got %d", report.JournalEntries)
	}
	if report.ReplayNodes != 4 {
		t.Errorf("expected 4 replay nodes, got %d", report.ReplayNodes)
	}
}

func TestReplay_MultipleRunsConsistent(t *testing.T) {
	rirPath := "../../test/fixtures/simple_dag.json"

	// Run 10 times — every replay must match
	for i := 0; i < 10; i++ {
		journalPath := "/tmp/mek_replay_multi_test.jsonl"
		recordJournal(t, rirPath, journalPath)

		report, err := Verify(context.Background(), rirPath, journalPath)
		os.Remove(journalPath)

		if err != nil {
			t.Fatal(err)
		}
		if !report.Match {
			t.Fatalf("run %d: replay diverged", i)
		}
	}
}

func TestReplay_DetectsDivergence(t *testing.T) {
	// Create a journal with intentionally wrong terminal status
	journalPath := "/tmp/mek_replay_divergence_test.jsonl"
	j, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	// Record a SUCCESS that should actually be FAILURE
	events <- commit.CommitEvent{
		NodeID:    "A",
		NewStatus: types.StatusSuccess,
	}
	close(events)
	<-done
	j.Close()
	defer os.Remove(journalPath)

	// Create a RIR where node A will fail
	failRIR := `{
  "meta": {"schema_version":"1.0","spec_hash":"replay-div-001","compilation_id":"rd","compiled_at":"2026-06-18T00:00:00Z","source_spec":"replay","compiler_version":"1.0.0"},
  "execution_plan": {"scheduling_model":"static_dag","execution_strategy":"dependency_first","max_parallelism":1,"fail_strategy":"fast","execution_mode":"2"},
  "units": [
    {"id":"A","type":"tool","binding":{"contract":"nonexistent_adapter","isolation":"inline"},"dependencies":[],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[],"optional":[]}}
  ],
  "graph":{"dag":{"nodes":["A"],"edges":[]},"cycles":[]},
  "assertions":[],"failure_modes":[]
}`
	rirPath := "/tmp/mek_replay_div_rir.json"
	os.WriteFile(rirPath, []byte(failRIR), 0644)
	defer os.Remove(rirPath)

	report, err := Verify(context.Background(), rirPath, journalPath)
	if err != nil {
		t.Fatal(err)
	}

	// Must detect the divergence
	if report.Match {
		t.Error("replay should have detected divergence (journal=SUCCESS, actual=FAILURE)")
	}
	if len(report.Divergences) == 0 {
		t.Error("expected at least one divergence")
	}
}

func TestReplay_RejectsMissingJournal(t *testing.T) {
	_, err := Verify(context.Background(),
		"../../test/fixtures/simple_dag.json",
		"/tmp/nonexistent_journal.jsonl")
	if err == nil {
		t.Error("should reject missing journal file")
	}
}

func recordJournal(t *testing.T, rirPath, journalPath string) {
	t.Helper()

	j, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	mek, err := runtime.New(rirPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan commit.CommitEvent, 1024)
	done := j.Subscribe(events)

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	sm := mek.StatusMap()
	for _, node := range mek.CEG().Nodes {
		state := sm.GetState(node.ID)
		if state != nil {
			events <- commit.CommitEvent{
				NodeID:    node.ID,
				NewStatus: state.Status,
				Outputs:   state.Outputs,
				Artifacts: state.Artifacts,
			}
		}
	}
	close(events)
	<-done
	_ = output
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
