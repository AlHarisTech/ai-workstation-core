package verify

import (
	"context"
	"os"
	"testing"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/journal"
	"github.com/anomalyco/mek/internal/replay"
	"github.com/anomalyco/mek/pkg/types"
)

func TestConsistency_AllDomainsAgree(t *testing.T) {
	report, err := FullConsistencyCheck(context.Background(), "../../test/fixtures/diamond_dag.json")
	if err != nil {
		t.Fatal(err)
	}

	if !report.Pass {
		t.Error("consistency check failed on valid diamond DAG")
	}
	for _, c := range report.Checks {
		if !c.Pass {
			t.Errorf("  [%s] FAIL: %s", c.Name, c.Detail)
		}
	}

	if len(report.Checks) != 4 {
		t.Errorf("expected 4 consistency checks, got %d", len(report.Checks))
	}
}

func TestConsistency_DetectsJournalTraceMismatch(t *testing.T) {
	journalPath := "/tmp/mek_consistency_div_test.jsonl"
	j, err := journal.New(journalPath)
	if err != nil {
		t.Fatal(err)
	}

	events := make(chan commit.CommitEvent, 10)
	done := j.Subscribe(events)

	events <- commit.CommitEvent{NodeID: "A", NewStatus: types.StatusSuccess}
	events <- commit.CommitEvent{NodeID: "B", NewStatus: types.StatusFailure}
	close(events)
	<-done
	j.Close()
	defer os.Remove(journalPath)

	rp, err := replay.Verify(context.Background(), "../../test/fixtures/simple_dag.json", journalPath)
	if err != nil {
		t.Fatal(err)
	}
	if rp.Match {
		t.Error("replay should detect journal/kernel mismatch")
	}
}

func TestConsistency_FullSpectrum(t *testing.T) {
	fixtures := []string{
		"../../test/fixtures/simple_dag.json",
		"../../test/fixtures/diamond_dag.json",
	}
	for _, f := range fixtures {
		t.Run(f, func(t *testing.T) {
			report, err := FullConsistencyCheck(context.Background(), f)
			if err != nil {
				t.Fatal(err)
			}
			if !report.Pass {
				t.Errorf("%s: consistency failed", f)
				for _, c := range report.Checks {
					t.Logf("  [%s] pass=%v detail=%s", c.Name, c.Pass, c.Detail)
				}
			}
		})
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
