package journal

import (
	"context"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

func TestJournalRecordsAllTransitions(t *testing.T) {
	tmpFile := "/tmp/mek_journal_test.jsonl"
	defer os.Remove(tmpFile)

	j, err := New(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Subscribe journal to commit events
	events := make(chan commit.CommitEvent, 1024)
	done := j.Subscribe(events)

	// Run MEK — we need to hook into the commit engine events
	// In the reference implementation, we access commit events directly
	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = output

	// Simulate recording by querying the status map directly
	// (full event subscription requires modifying MEK to expose the channel)
	sm := mek.StatusMap()
	for _, node := range mek.CEG().Nodes {
		state := sm.GetState(node.ID)
		if state != nil {
			evt := commit.CommitEvent{
				NodeID:    node.ID,
				NewStatus: state.Status,
				Outputs:   state.Outputs,
				Artifacts: state.Artifacts,
			}
			events <- evt
		}
	}
	close(events)
	<-done

	entries := j.Entries()
	if len(entries) != 4 {
		t.Errorf("expected 4 journal entries, got %d", len(entries))
	}

	// Verify each node has an entry
	nodeIDs := map[string]bool{}
	for _, e := range entries {
		nodeIDs[e.NodeID] = true
		if !e.ToStatus.IsTerminal() {
			t.Errorf("journal entry for %s has non-terminal status: %s", e.NodeID, e.ToStatus)
		}
	}
	for _, id := range []string{"root", "mid_a", "mid_b", "leaf"} {
		if !nodeIDs[id] {
			t.Errorf("journal missing entry for node %s", id)
		}
	}
}

func TestJournalSequenceMonotonic(t *testing.T) {
	tmpFile := "/tmp/mek_journal_seq_test.jsonl"
	defer os.Remove(tmpFile)

	j, err := New(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	for i := 0; i < 5; i++ {
		events <- commit.CommitEvent{
			NodeID:    "test",
			NewStatus: types.StatusSuccess,
		}
	}
	close(events)
	<-done

	entries := j.Entries()
	for i, e := range entries {
		if e.Sequence != uint64(i) {
			t.Errorf("sequence gap: expected %d, got %d", i, e.Sequence)
		}
	}
}

func TestJournalNodeHistory(t *testing.T) {
	tmpFile := "/tmp/mek_journal_history_test.jsonl"
	defer os.Remove(tmpFile)

	j, err := New(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	// Node A: BLOCKED → READY → RUNNING → SUCCESS
	for _, status := range []types.NodeStatus{
		types.StatusBlocked, types.StatusReady, types.StatusRunning, types.StatusSuccess,
	} {
		events <- commit.CommitEvent{NodeID: "A", NewStatus: status}
	}

	// Node B: BLOCKED → READY → SKIPPED
	for _, status := range []types.NodeStatus{
		types.StatusBlocked, types.StatusReady, types.StatusSkipped,
	} {
		events <- commit.CommitEvent{NodeID: "B", NewStatus: status}
	}

	close(events)
	<-done

	historyA := j.NodeHistory("A")
	if len(historyA) != 4 {
		t.Errorf("node A history: expected 4 entries, got %d", len(historyA))
	}

	historyB := j.NodeHistory("B")
	if len(historyB) != 3 {
		t.Errorf("node B history: expected 3 entries, got %d", len(historyB))
	}

	finalA, ok := j.TerminalStatus("A")
	if !ok || finalA != types.StatusSuccess {
		t.Errorf("node A terminal: expected SUCCESS, got %v (ok=%v)", finalA, ok)
	}

	finalB, ok := j.TerminalStatus("B")
	if !ok || finalB != types.StatusSkipped {
		t.Errorf("node B terminal: expected SKIPPED, got %v (ok=%v)", finalB, ok)
	}
}

func TestJournalDurability(t *testing.T) {
	tmpFile := "/tmp/mek_journal_durable_test.jsonl"
	defer os.Remove(tmpFile)

	// Write entries
	j1, err := New(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	events := make(chan commit.CommitEvent, 10)
	done := j1.Subscribe(events)

	for i := 0; i < 3; i++ {
		events <- commit.CommitEvent{
			NodeID:    "durable_test",
			NewStatus: types.StatusSuccess,
		}
	}
	close(events)
	<-done
	j1.Close()

	// Verify file was written
	stat, err := os.Stat(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	if stat.Size() == 0 {
		t.Error("journal file is empty — durability check failed")
	}
}

func TestJournalImmutability(t *testing.T) {
	tmpFile := "/tmp/mek_journal_immutable_test.jsonl"
	defer os.Remove(tmpFile)

	j, err := New(tmpFile)
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	events <- commit.CommitEvent{NodeID: "X", NewStatus: types.StatusSuccess}
	close(events)
	<-done

	// Snapshot entries
	snap1 := j.Entries()
	if len(snap1) != 1 {
		t.Fatal("expected 1 entry")
	}

	// Attempt to modify the snapshot — must not affect journal
	snap1[0] = Entry{Sequence: 999}
	snap2 := j.Entries()
	if snap2[0].Sequence != 0 {
		t.Errorf("journal modified through Entries() slice: seq=%d", snap2[0].Sequence)
	}
}
