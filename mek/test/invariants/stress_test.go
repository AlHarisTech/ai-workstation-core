package invariants

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

// generateDAG creates a linear chain of N nodes: 0 → 1 → 2 → ... → N-1
func generateLinearRIR(n int) string {
	var units, edges strings.Builder
	units.WriteString(`[`)
	edges.WriteString(`[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			units.WriteString(",")
		}
		units.WriteString(fmt.Sprintf(
			`{"id":"n%d","type":"tool","binding":{"contract":"internal/noop","isolation":"inline"},"dependencies":[`, i))
		if i > 0 {
			units.WriteString(fmt.Sprintf(`"n%d"`, i-1))
		}
		units.WriteString(`],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[`)
		if i > 0 {
			units.WriteString(fmt.Sprintf(`"n%d"`, i-1))
		}
		units.WriteString(`],"optional":[]}}`)

		if i > 0 {
			if i > 1 {
				edges.WriteString(",")
			}
			edges.WriteString(fmt.Sprintf(
				`{"from":"n%d","to":"n%d","type":"dependency"}`, i-1, i))
		}
	}
	units.WriteString(`]`)
	edges.WriteString(`]`)

	var allNodeIDs strings.Builder
	allNodeIDs.WriteString(`[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			allNodeIDs.WriteString(",")
		}
		allNodeIDs.WriteString(fmt.Sprintf(`"n%d"`, i))
	}
	allNodeIDs.WriteString(`]`)

	return fmt.Sprintf(`{
  "meta": {"schema_version":"1.0","spec_hash":"stress-linear-%d","compilation_id":"stress-%d","compiled_at":"2026-06-18T00:00:00Z","source_spec":"stress","compiler_version":"1.0.0"},
  "execution_plan": {"scheduling_model":"static_dag","execution_strategy":"dependency_first","max_parallelism":4,"fail_strategy":"fast","execution_mode":"2"},
  "units": %s,
  "graph": {"dag": {"nodes": %s, "edges": %s}, "cycles": []},
  "assertions": [], "failure_modes": []
}`, n, n, units.String(), allNodeIDs.String(), edges.String())
}

func TestStress_Linear10(t *testing.T) {
	testLinearN(t, 10)
}

func TestStress_Linear100(t *testing.T) {
	testLinearN(t, 100)
}

func TestStress_Linear1000(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 1000-node stress test in short mode")
	}

	// 1000-node linear DAG exceeds max depth (128) — MEK must reject it.
	rir := generateLinearRIR(1000)
	tmpFile := fmt.Sprintf("/tmp/mek_stress_linear_1000_%d.json", os.Getpid())
	os.WriteFile(tmpFile, []byte(rir), 0644)
	defer os.Remove(tmpFile)

	_, err := runtime.New(tmpFile, nil)
	if err == nil {
		t.Fatal("1000-node linear DAG should have been rejected (depth > 128)")
	}
	if !strings.Contains(err.Error(), "MAX_DEPTH") && !strings.Contains(err.Error(), "depth") {
		t.Logf("rejection error (non-standard message): %v", err)
	}
}

func TestStress_Deep128(t *testing.T) {
	// Exactly at the boundary — should pass.
	testLinearN(t, 128)
}

func TestStress_Deep129(t *testing.T) {
	// One over the boundary — should be rejected.
	rir := generateLinearRIR(129)
	tmpFile := fmt.Sprintf("/tmp/mek_stress_linear_129_%d.json", os.Getpid())
	os.WriteFile(tmpFile, []byte(rir), 0644)
	defer os.Remove(tmpFile)

	_, err := runtime.New(tmpFile, nil)
	if err == nil {
		t.Fatal("129-node linear DAG should have been rejected (depth > 128)")
	}
}

func testLinearN(t *testing.T, n int) {
	rir := generateLinearRIR(n)
	tmpFile := fmt.Sprintf("/tmp/mek_stress_linear_%d_%d.json", n, os.Getpid())
	if err := os.WriteFile(tmpFile, []byte(rir), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	mek, err := runtime.New(tmpFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify CEG depth
	depth := len(mek.CEG().Layers)
	if depth != n {
		t.Errorf("expected %d layers, got %d", n, depth)
	}
	if depth > 128 {
		t.Errorf("depth %d exceeds max 128", depth)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// All nodes must be terminal
	for _, node := range mek.CEG().Nodes {
		state := output.StatusMap[node.ID]
		if state == nil || !state.Status.IsTerminal() {
			t.Errorf("node %s not terminal in %d-node DAG", node.ID, n)
		}
	}
}

// generateWideDAG creates N independent root nodes (no edges, all parallel)
func generateWideRIR(n int) string {
	var units strings.Builder
	units.WriteString(`[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			units.WriteString(",")
		}
		units.WriteString(fmt.Sprintf(
			`{"id":"n%d","type":"tool","binding":{"contract":"internal/noop","isolation":"inline"},"dependencies":[],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[],"optional":[]}}`, i))
	}
	units.WriteString(`]`)

	var allNodeIDs strings.Builder
	allNodeIDs.WriteString(`[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			allNodeIDs.WriteString(",")
		}
		allNodeIDs.WriteString(fmt.Sprintf(`"n%d"`, i))
	}
	allNodeIDs.WriteString(`]`)

	return fmt.Sprintf(`{
  "meta": {"schema_version":"1.0","spec_hash":"stress-wide-%d","compilation_id":"stress-wide-%d","compiled_at":"2026-06-18T00:00:00Z","source_spec":"stress","compiler_version":"1.0.0"},
  "execution_plan": {"scheduling_model":"static_dag","execution_strategy":"parallel_first","max_parallelism":%d,"fail_strategy":"continue","execution_mode":"2"},
  "units": %s,
  "graph": {"dag": {"nodes": %s, "edges": []}, "cycles": []},
  "assertions": [], "failure_modes": []
}`, n, n, n, units.String(), allNodeIDs.String())
}

func TestStress_Wide100(t *testing.T) {
	testWideN(t, 100)
}

func TestStress_Wide500(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 500-node wide stress test in short mode")
	}
	testWideN(t, 500)
}

func testWideN(t *testing.T, n int) {
	rir := generateWideRIR(n)
	tmpFile := fmt.Sprintf("/tmp/mek_stress_wide_%d_%d.json", n, os.Getpid())
	if err := os.WriteFile(tmpFile, []byte(rir), 0644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile)

	mek, err := runtime.New(tmpFile, nil)
	if err != nil {
		t.Fatal(err)
	}

	output, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	successCount := 0
	for _, node := range mek.CEG().Nodes {
		state := output.StatusMap[node.ID]
		if state != nil && state.Status == types.StatusSuccess {
			successCount++
		}
	}
	if successCount != n {
		t.Errorf("expected %d SUCCESS nodes, got %d", n, successCount)
	}
}

func TestStress_InvalidRIR_Rejection(t *testing.T) {
	invalidRIR := `{"meta":{"schema_version":"1.0","spec_hash":"","compilation_id":"x","compiled_at":"2026-06-18T00:00:00Z","source_spec":"x","compiler_version":"1.0.0"},"execution_plan":{"scheduling_model":"static_dag","execution_strategy":"dependency_first","max_parallelism":1,"fail_strategy":"fast","execution_mode":"2"},"units":[{"id":"x","type":"tool","binding":{"contract":"unknown","isolation":"inline"},"dependencies":[],"data_flow":{"outputs":[]},"validation":{"preconditions":[],"postconditions":[],"invariants":[],"failure_modes":[]},"scheduling":{"priority":1},"context":{"mode":"fresh","tools":[]},"governance":{"required_approvals":[],"change_scope":"read_only"},"activation":{"condition":"all_success","requires":[],"optional":[]}}],"graph":{"dag":{"nodes":["x"],"edges":[]},"cycles":[]},"assertions":[],"failure_modes":[]}`

	// Missing spec_hash => should be rejected
	tmpFile := "/tmp/mek_invalid_rir.json"
	os.WriteFile(tmpFile, []byte(invalidRIR), 0644)
	defer os.Remove(tmpFile)

	_, err := runtime.New(tmpFile, nil)
	if err == nil {
		t.Error("MISSING_SPEC_HASH should have been rejected")
	}
}

// ─── Race detection helpers ───
// Run with: go test -race ./test/invariants/

func TestRace_ConcurrentWaves(t *testing.T) {
	// A wide DAG with many parallel nodes exercises the concurrent dispatch path
	testWideN(t, 100)
}

func TestRace_RepeatedRuns(t *testing.T) {
	// Run the same DAG from multiple goroutines simultaneously
	done := make(chan bool, 5)
	for i := 0; i < 5; i++ {
		go func() {
			mek, err := runtime.New("../../test/fixtures/diamond_dag.json", nil)
			if err != nil {
				t.Error(err)
				done <- false
				return
			}
			_, err = mek.Run(context.Background())
			if err != nil {
				t.Error(err)
				done <- false
				return
			}
			done <- true
		}()
	}
	for i := 0; i < 5; i++ {
		if !<-done {
			t.Error("concurrent run failed")
		}
	}
}

// ─── Import needed for type checking in wide test ───
var _ = json.Marshal // keep json import
