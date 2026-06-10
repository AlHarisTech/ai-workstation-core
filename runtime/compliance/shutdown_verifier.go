package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
)

func VerifyShutdown(t *testing.T) ShutdownComplianceReport {
	t.Helper()
	report := ShutdownComplianceReport{
		Timestamp:       time.Now(),
		CompliancePass:  true,
	}

	cfg := kernel.DefaultConfig(); ws := tempWorkspaceRoot(); defer os.RemoveAll(ws); cfg.WorkspaceRoot = ws
	cfg.WorkerCount = 2
	harden := kernel.DefaultHardeningConfig()

	engine, err := kernel.NewKernelEngine(cfg, harden)
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}
	engine.Start()

	report.RequestsInflight = 10
	for i := 0; i < report.RequestsInflight; i++ {
		input := fmt.Sprintf(`{"id":"sd_%d","method":"tool.call","params":{"tool":"echo","arguments":{"msg":"shutdown"}},"session":{"session_id":"s1","project_id":"p1"}}`, i)
		if err := engine.Ingest(json.RawMessage(input)); err != nil {
			report.RequestsInflight--
		}
	}

	time.Sleep(300 * time.Millisecond)

	engine.GracefulShutdown()

	result, err := engine.ReplayHistory()
	if err != nil {
		report.StateFlushSuccess = false
		report.CompliancePass = false
		return report
	}

	report.RequestsCompleted = result.TotalCount
	report.RequestsLost = report.RequestsInflight - report.RequestsCompleted
	if report.RequestsLost < 0 {
		report.RequestsLost = 0
	}

	report.StateFlushSuccess = true

	if report.RequestsLost > 0 {
		report.CompliancePass = false
	}

	return report
}
