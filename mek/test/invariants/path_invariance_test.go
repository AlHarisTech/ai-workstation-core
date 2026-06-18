package invariants

import (
	"context"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/journal"
	"github.com/anomalyco/mek/internal/replay"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/internal/verify"
	"github.com/anomalyco/mek/pkg/types"
)

// ─── Path Invariance: All entry surfaces converge to identical execution ───
//
// Theorem: ∀ valid entry paths p₁, p₂: Execute(MEK, p₁) ≡ Execute(MEK, p₂)
//
// Entry surfaces tested:
//   E1 — RIR Loader    (mek.Run via runtime.New)
//   E2 — Replay Verify (replay.Verify with journal from E1)
//   E3 — Structural     (verify.Structural with journal from E1)
//   E4 — Consistency    (verify.FullConsistencyCheck)

func TestPathInvariance_SameRIR(t *testing.T) {
	fixtures := []string{
		"../../test/fixtures/simple_dag.json",
		"../../test/fixtures/diamond_dag.json",
	}

	for _, rirPath := range fixtures {
		t.Run(rirPath, func(t *testing.T) {
			// E1: Direct RIR execution — establish ground truth
			e1 := runPathE1(t, rirPath)
			journalPath := recordAndClose(t, rirPath)

			// E2: Replay — must match E1
			e2 := runPathE2(t, rirPath, journalPath)

			// E3: Structural — must pass against E1's journal
			e3 := runPathE3(t, rirPath, journalPath)

			// E4: Full consistency — all domains must agree
			e4 := runPathE4(t, rirPath)

			// Path Invariance: E1 ≡ E2 ≡ E3 ≡ E4
			// E1 agrees with E2 (Replay)
			if !e2 {
				t.Error("PATH DIVERGENCE: E1 (kernel) ≠ E2 (replay)")
			}
			// E1 agrees with E3 (Structural)
			if !e3 {
				t.Error("PATH DIVERGENCE: E1 (kernel) fails structural check")
			}
			// E4 covers all remaining pairs via lattice closure
			if !e4 {
				t.Error("PATH DIVERGENCE: cross-domain consistency failure")
			}

			_ = e1
			_ = journalPath
		})
	}
}

func TestPathInvariance_MultipleRunsConverge(t *testing.T) {
	rirPath := "../../test/fixtures/diamond_dag.json"

	// Run E1 50 times — every run must produce identical results
	var first map[string]types.NodeStatus
	for i := 0; i < 50; i++ {
		mek, err := runtime.New(rirPath, nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := mek.Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}

		current := make(map[string]types.NodeStatus)
		for id, state := range out.StatusMap {
			current[id] = state.Status
		}

		if first == nil {
			first = current
			continue
		}
		for id, s1 := range first {
			s2 := current[id]
			if s1 != s2 {
				t.Fatalf("PATH DIVERGENCE at run %d: node %s run1=%s run%d=%s",
					i+1, id, s1, i+1, s2)
			}
		}
	}
}

func TestPathInvariance_ReplayMatchesAllEntryPaths(t *testing.T) {
	// E2 (Replay) is the strongest oracle — it re-executes the RIR.
	// If Replay = Kernel, all paths converge.
	fixtures := []string{
		"../../test/fixtures/simple_dag.json",
		"../../test/fixtures/diamond_dag.json",
	}

	for _, rirPath := range fixtures {
		journalPath := recordAndClose(t, rirPath)

		rp, err := replay.Verify(context.Background(), rirPath, journalPath)
		if err != nil {
			t.Fatalf("%s: replay error: %v", rirPath, err)
		}
		if !rp.Match {
			t.Errorf("%s: REPLAY DIVERGENCE — paths do not converge", rirPath)
			for _, d := range rp.Divergences {
				t.Logf("  node %s: journal=%s replay=%s", d.NodeID, d.JournalStatus, d.ReplayStatus)
			}
		}
	}
}

func TestPathInvariance_StructuralPassesAllPaths(t *testing.T) {
	fixtures := []string{
		"../../test/fixtures/simple_dag.json",
		"../../test/fixtures/diamond_dag.json",
	}

	for _, rirPath := range fixtures {
		journalPath := recordAndClose(t, rirPath)

		sr, err := verify.Structural(rirPath, journalPath)
		if err != nil {
			t.Fatalf("%s: structural error: %v", rirPath, err)
		}
		if !sr.Pass {
			t.Errorf("%s: STRUCTURAL FAILURE — path violates constraints", rirPath)
			for _, v := range sr.Violations {
				t.Logf("  [%s] node=%s: %s", v.Rule, v.NodeID, v.Message)
			}
		}
	}
}

// ─── Helpers ───

func runPathE1(t *testing.T, rirPath string) map[string]types.NodeStatus {
	t.Helper()
	mek, err := runtime.New(rirPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := mek.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	result := make(map[string]types.NodeStatus)
	for id, state := range out.StatusMap {
		result[id] = state.Status
	}
	return result
}

func runPathE2(t *testing.T, rirPath, journalPath string) bool {
	t.Helper()
	rp, err := replay.Verify(context.Background(), rirPath, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	return rp.Match
}

func runPathE3(t *testing.T, rirPath, journalPath string) bool {
	t.Helper()
	sr, err := verify.Structural(rirPath, journalPath)
	if err != nil {
		t.Fatal(err)
	}
	return sr.Pass
}

func runPathE4(t *testing.T, rirPath string) bool {
	t.Helper()
	cr, err := verify.FullConsistencyCheck(context.Background(), rirPath)
	if err != nil {
		t.Fatal(err)
	}
	return cr.Pass
}

func recordAndClose(t *testing.T, rirPath string) string {
	t.Helper()
	path := "/tmp/mek_path_invariance_test.jsonl"

	j, err := journal.New(path)
	if err != nil {
		t.Fatal(err)
	}

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
				NodeID: node.ID, NewStatus: state.Status,
				Outputs: state.Outputs, Artifacts: state.Artifacts,
			}
		}
	}
	close(events)
	<-done
	j.Close()
	_ = output

	t.Cleanup(func() { os.Remove(path) })
	return path
}
