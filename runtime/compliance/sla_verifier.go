package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
	"github.com/AlHarisTech/ai-workstation-core/runtime/observability"
)

func VerifySLA(t *testing.T) SLAComplianceReport {
	t.Helper()
	report := SLAComplianceReport{
		Timestamp:       time.Now(),
		CompliancePass:  true,
	}

	cfg := kernel.DefaultConfig(); ws := tempWorkspaceRoot(); defer os.RemoveAll(ws); cfg.WorkspaceRoot = ws
	cfg.WorkerCount = 4
	harden := kernel.DefaultHardeningConfig()

	engine, err := kernel.NewKernelEngine(cfg, harden)
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}
	engine.Start()

	tracker := observability.NewLatencyTracker(2000)

	counts := []int{100}
	for _, count := range counts {
		for i := 0; i < count; i++ {
			input := fmt.Sprintf(`{"id":"sla_%d","method":"tool.call","params":{"tool":"echo","arguments":{"msg":"sla"}},"session":{"session_id":"s1","project_id":"p1"}}`, i)
			engine.Ingest(json.RawMessage(input))
		}
		time.Sleep(time.Duration(count/10+100) * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)

	sla := tracker.Snapshot()

	report.P50 = sla.P50
	report.P95 = sla.P95
	report.P99 = sla.P99
	report.Max = sla.Max

	thresh := DefaultSLA
	violations := 0
	if sla.P50 > thresh.P50Max {
		report.SLAViolation = true
		violations++
	}
	if sla.P95 > thresh.P95Max {
		report.SLAViolation = true
		violations++
	}
	if sla.P99 > thresh.P99Max {
		report.SLAViolation = true
		violations++
	}
	report.Violations = violations

	if report.SLAViolation {
		report.CompliancePass = false
	}

	engine.GracefulShutdown()
	return report
}
