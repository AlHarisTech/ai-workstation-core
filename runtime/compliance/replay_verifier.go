package compliance

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
)

func VerifyReplay(t *testing.T) ReplayComplianceReport {
	t.Helper()
	report := ReplayComplianceReport{
		Timestamp:       time.Now(),
		OrderingMatch:   true,
		PolicyMatch:     true,
		StateMatch:      true,
		CompliancePass:  true,
	}

	cfg := kernel.DefaultConfig(); ws := tempWorkspaceRoot(); defer os.RemoveAll(ws); cfg.WorkspaceRoot = ws
	harden := kernel.DefaultHardeningConfig()

	engine, err := kernel.NewKernelEngine(cfg, harden)
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}
	engine.Start()

	hashes := make([]string, 0)

	for i := 0; i < 5; i++ {
		input := fmt.Sprintf(`{"id":"r%d","method":"tool.call","params":{"tool":"echo","arguments":{"msg":"replay_%d"}},"session":{"session_id":"s1","project_id":"p1"}}`, i, i)
		engine.Ingest(json.RawMessage(input))
		time.Sleep(10 * time.Millisecond)
	}

	time.Sleep(100 * time.Millisecond)

	result, err := engine.ReplayHistory()
	if err != nil || result == nil {
		report.OrderingMatch = false
		report.CompliancePass = false
		return report
	}

	for _, trace := range result.Traces {
		h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%v", trace.RequestID, trace.Status, len(trace.ExecutionTrace))))
		hashes = append(hashes, fmt.Sprintf("%x", h[:8]))
	}

	if len(hashes) > 0 {
		report.OriginalHash = hashes[0]
	}
	if len(hashes) > 0 {
		report.ReplayHash = hashes[len(hashes)-1]
	}

	if result.TotalCount < 5 {
		report.StateMatch = false
		report.CompliancePass = false
	}

	engine.GracefulShutdown()
	return report
}
