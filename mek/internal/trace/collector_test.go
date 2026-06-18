package trace

import (
	"context"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

func TestCollectorRecordsAllNodes(t *testing.T) {
	collector := NewCollector()

	mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Run MEK and feed commit events to collector
	events := make(chan commit.CommitEvent, 1024)
	traceEvents := collector.Subscribe(events)

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	_ = output

	// Collect terminal events from status map
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

	// Drain trace events
	eventCount := 0
	for range traceEvents {
		eventCount++
	}

	if eventCount == 0 {
		t.Error("no trace events emitted")
	}

	// Every CEG node should have a trace
	for _, node := range mek.CEG().Nodes {
		trace, ok := collector.Trace(node.ID)
		if !ok {
			t.Errorf("missing trace for node %s", node.ID)
			continue
		}
		if !trace.TerminalStatus.IsTerminal() {
			t.Errorf("trace for %s has non-terminal status: %s", node.ID, trace.TerminalStatus)
		}
	}

	// AllTraces should have all nodes
	all := collector.AllTraces()
	if len(all) != len(mek.CEG().Nodes) {
		t.Errorf("AllTraces: expected %d, got %d", len(mek.CEG().Nodes), len(all))
	}
}

func TestCollectorTerminalStatuses(t *testing.T) {
	collector := NewCollector()
	events := make(chan commit.CommitEvent, 10)
	done := collector.Subscribe(events)

	events <- commit.CommitEvent{NodeID: "A", NewStatus: types.StatusSuccess}
	events <- commit.CommitEvent{NodeID: "B", NewStatus: types.StatusFailure}
	events <- commit.CommitEvent{NodeID: "C", NewStatus: types.StatusSkipped}
	events <- commit.CommitEvent{NodeID: "D", NewStatus: types.StatusTerminated}
	close(events)
	<-done

	statuses := collector.TerminalStatuses()
	if len(statuses) != 4 {
		t.Errorf("expected 4 terminal statuses, got %d", len(statuses))
	}
	if statuses["A"] != types.StatusSuccess {
		t.Errorf("A: expected SUCCESS, got %s", statuses["A"])
	}
	if statuses["B"] != types.StatusFailure {
		t.Errorf("B: expected FAILURE, got %s", statuses["B"])
	}
	if statuses["C"] != types.StatusSkipped {
		t.Errorf("C: expected SKIPPED, got %s", statuses["C"])
	}
	if statuses["D"] != types.StatusTerminated {
		t.Errorf("D: expected TERMINATED, got %s", statuses["D"])
	}
}

func TestCollectorPassiveNonIntrusive(t *testing.T) {
	// OB-001: Observability never affects execution.
	// Run MEK with and without the trace collector — results must be identical.

	runWithoutCollector := func() map[string]types.NodeStatus {
		mek, _ := runtime.New("../../test/fixtures/simple_dag.json", nil)
		out, _ := mek.Run(context.Background())
		result := make(map[string]types.NodeStatus)
		for id, state := range out.StatusMap {
			result[id] = state.Status
		}
		return result
	}

	runWithCollector := func() map[string]types.NodeStatus {
		mek, _ := runtime.New("../../test/fixtures/simple_dag.json", nil)
		collector := NewCollector()
		events := make(chan commit.CommitEvent, 1024)
		_ = collector.Subscribe(events)
		out, _ := mek.Run(context.Background())
		sm := mek.StatusMap()
		for _, node := range mek.CEG().Nodes {
			state := sm.GetState(node.ID)
			if state != nil {
				events <- commit.CommitEvent{
					NodeID:    node.ID,
					NewStatus: state.Status,
				}
			}
		}
		close(events)
		result := make(map[string]types.NodeStatus)
		for id, state := range out.StatusMap {
			result[id] = state.Status
		}
		return result
	}

	// Run both 50 times — must be identical every time
	for i := 0; i < 50; i++ {
		r1 := runWithoutCollector()
		r2 := runWithCollector()
		for id := range r1 {
			if r1[id] != r2[id] {
				t.Fatalf("OB-001 VIOLATED: node %s without=%s with=%s", id, r1[id], r2[id])
			}
		}
	}
}

func TestCollectorTraceImmutability(t *testing.T) {
	collector := NewCollector()
	events := make(chan commit.CommitEvent, 10)
	done := collector.Subscribe(events)

	events <- commit.CommitEvent{
		NodeID:    "X",
		NewStatus: types.StatusSuccess,
		Outputs:   map[string]interface{}{"key": "value"},
	}
	close(events)
	// Drain all trace events before checking
	for range done {
	}

	// Get trace and attempt to mutate it
	trace1, ok := collector.Trace("X")
	if !ok {
		t.Fatal("trace not found")
	}
	if trace1.Outputs != nil {
		trace1.Outputs["key"] = "mutated"
	}

	// Get trace again — must be unchanged
	trace2, _ := collector.Trace("X")
	if trace2.Outputs == nil || trace2.Outputs["key"] != "value" {
		t.Errorf("trace mutated through snapshot: got %v", trace2.Outputs)
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
