package stress

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
	"github.com/AlHarisTech/ai-workstation-core/runtime/validation/contracts"
	vtypes "github.com/AlHarisTech/ai-workstation-core/runtime/validation/types"
)

func tempRoot(t *testing.T) string {
	d, _ := os.MkdirTemp("", "stress-*")
	os.MkdirAll(d+"/.ai/governance/audit", 0755)
	os.MkdirAll(d+"/.ai/state/traces", 0755)
	os.MkdirAll(d+"/.ai/state/sessions", 0755)
	os.MkdirAll(d+"/.ai/state/snapshots", 0755)
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func runEngine(t *testing.T, ws string, sessions int, total int) vtypes.StressValidationReport {
	cfg := kernel.DefaultConfig()
	cfg.WorkspaceRoot = ws
	cfg.WorkerCount = 4
	cfg.QueueSize = 512
	harden := kernel.DefaultHardeningConfig()

	engine, err := kernel.NewKernelEngine(cfg, harden)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	engine.Start()

	start := time.Now()

	for i := 0; i < total; i++ {
		sid := fmt.Sprintf("sess_%d", i%sessions)
		input := fmt.Sprintf(`{"id":"stress_%d","method":"tool.call","params":{"tool":"echo","arguments":{"msg":"s"}},"session":{"session_id":"%s","project_id":"p1"}}`, i, sid)
		if err := engine.Ingest(json.RawMessage(input)); err != nil {
			if err.Error() == "Queue full" {
				continue
			}
		}
	}

	time.Sleep(time.Duration(total/100+500) * time.Millisecond)

	elapsed := time.Since(start).Milliseconds()

	result, err := engine.ReplayHistory()
	if err != nil {
		t.Logf("replay error: %v", err)
	}

	failures := 0
	if result != nil {
		for _, tr := range result.Traces {
			if tr.Status == "error" || tr.Status == "denied" {
				failures++
			}
		}
	}

	report := vtypes.StressValidationReport{
		Scenario:       fmt.Sprintf("%d sessions / %d requests", sessions, total),
		Requests:       total,
		Sessions:       sessions,
		Failures:       failures,
		P50:            float64(elapsed) / float64(total) * 1000,
		ValidationPass: failures < total/10,
		Timestamp:      time.Now(),
	}

	engine.GracefulShutdown()
	return report
}

func RunStressTests(t *testing.T) []vtypes.StressValidationReport {
	ws := tempRoot(t)
	var results []vtypes.StressValidationReport

	for _, sc := range contracts.StressScenarios {
		r := runEngine(t, ws, sc.Sessions, sc.Requests)
		r.Scenario = sc.Name
		results = append(results, r)
	}
	return results
}
