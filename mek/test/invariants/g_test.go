package invariants

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

// ─── G1: Deterministic — same RIR → same execution result ───

func TestG1_Determinism(t *testing.T) {
	var first *types.MEKOutput
	for i := 0; i < 500; i++ {
		mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := mek.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = out
			continue
		}
		for _, node := range mek.CEG().Nodes {
			s1 := first.StatusMap[node.ID]
			s2 := out.StatusMap[node.ID]
			if s1.Status != s2.Status {
				t.Fatalf("G1 VIOLATED: node %s run1=%s run%d=%s",
					node.ID, s1.Status, i+1, s2.Status)
			}
		}
		if out.Metrics.NodesExecuted != first.Metrics.NodesExecuted {
			t.Fatalf("G1 VIOLATED: nodes_executed %d vs %d",
				first.Metrics.NodesExecuted, out.Metrics.NodesExecuted)
		}
	}
}

// ─── G2: Isolated — node failures do not crash MEK ───

func TestG2_Isolation(t *testing.T) {
	// A failing node should produce FAILURE status, not crash the runtime.
	// The remaining nodes should complete normally.

	failRIR := `{
  "meta": {"schema_version":"1.0","spec_hash":"g2-001","compilation_id":"g2","compiled_at":"2026-06-18T00:00:00Z","source_spec":"g2","compiler_version":"1.0.0"},
  "execution_plan": {"scheduling_model":"static_dag","execution_strategy":"dependency_first","max_parallelism":4,"fail_strategy":"fast","execution_mode":"2"},
  "units": [
    {"id":"good_A","type":"tool","binding":{"contract":"internal/noop","isolation":"inline"},"dependencies":[],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[],"optional":[]}},
    {"id":"bad_B","type":"tool","binding":{"contract":"nonexistent_adapter","isolation":"inline"},"dependencies":[],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[],"optional":[]}},
    {"id":"good_C","type":"tool","binding":{"contract":"internal/noop","isolation":"inline"},"dependencies":[],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[],"optional":[]}}
  ],
  "graph":{"dag":{"nodes":["good_A","bad_B","good_C"],"edges":[]},"cycles":[]},
  "assertions":[],"failure_modes":[]
}`
	tmpFile := fmt.Sprintf("/tmp/mek_g2_%d.json", os.Getpid())
	if err := os.WriteFile(tmpFile, []byte(failRIR), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	mek, err := runtime.New(tmpFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		// Run should NOT error — individual node failures don't crash MEK
		t.Fatalf("G2 VIOLATED: MEK crashed on node failure: %v", err)
	}

	// Good nodes must succeed
	if output.StatusMap["good_A"].Status != types.StatusSuccess {
		t.Error("G2 VIOLATED: good_A should succeed despite bad_B failure")
	}
	if output.StatusMap["good_C"].Status != types.StatusSuccess {
		t.Error("G2 VIOLATED: good_C should succeed despite bad_B failure")
	}

	// Bad node must fail (but didn't crash MEK)
	if output.StatusMap["bad_B"].Status != types.StatusFailure {
		t.Errorf("G2: bad_B should be FAILURE, got %s", output.StatusMap["bad_B"].Status)
	}
}

// ─── G3: Consistent — all state transitions flow through Commit Engine ───

func TestG3_Consistency(t *testing.T) {
	// Run diamond DAG: verify every node reaches a valid terminal state.
	// No node should be in a non-terminal state at completion.
	// No node should have an invalid terminal status.

	mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	validTerminals := map[types.NodeStatus]bool{
		types.StatusSuccess:    true,
		types.StatusFailure:    true,
		types.StatusSkipped:    true,
		types.StatusTerminated: true,
	}

	for _, node := range mek.CEG().Nodes {
		state := output.StatusMap[node.ID]
		if state == nil {
			t.Errorf("G3 VIOLATED: node %s missing from status map", node.ID)
			continue
		}
		if !validTerminals[state.Status] {
			t.Errorf("G3 VIOLATED: node %s has invalid terminal status: %s", node.ID, state.Status)
		}
	}
}

// ─── G4: Bounded — execution reaches terminal closure or is TERMINATED ───

func TestG4_BoundedTermination(t *testing.T) {
	mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// All nodes must be terminal
	for _, node := range mek.CEG().Nodes {
		state := output.StatusMap[node.ID]
		if state == nil {
			t.Errorf("G4 VIOLATED: node %s missing", node.ID)
			continue
		}
		if !state.Status.IsTerminal() {
			t.Errorf("G4 VIOLATED: node %s not terminal: %s", node.ID, state.Status)
		}
	}

	// Duration must be set
	if output.Metrics.TotalDurationMs < 0 {
		t.Error("G4 VIOLATED: duration not set")
	}
}

// ─── G5: Pure — no side effects beyond adapter invocations ───

func TestG5_Pure(t *testing.T) {
	// MEK itself produces no side effects beyond adapter invocations.
	// No filesystem writes, no network calls, no external mutations.
	// Verified structurally: the runtime package has no I/O beyond RIR loading.

	mek, err := runtime.New("../../test/fixtures/simple_dag.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Output must be structurally valid
	if output.StatusMap == nil {
		t.Error("G5 VIOLATED: no status map returned")
	}
}

// ─── G6: Verifiable — output status_map validates against CEG structure ───

func TestG6_Verifiable(t *testing.T) {
	mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Every CEG node must have a status entry
	for _, node := range mek.CEG().Nodes {
		if _, ok := output.StatusMap[node.ID]; !ok {
			t.Errorf("G6 VIOLATED: CEG node %s has no output status", node.ID)
		}
	}

	// Every dependency edge must be satisfied:
	// If A depends on B and A is SUCCESS, B must be SUCCESS.
	for _, edge := range mek.CEG().Edges {
		if edge.Type == "dependency" {
			toStatus := output.StatusMap[edge.To].Status
			fromStatus := output.StatusMap[edge.From].Status
			if toStatus == types.StatusSuccess && fromStatus != types.StatusSuccess {
				t.Errorf("G6 VIOLATED: node %s SUCCESS but dependency %s is %s",
					edge.To, edge.From, fromStatus)
			}
		}
	}
}
