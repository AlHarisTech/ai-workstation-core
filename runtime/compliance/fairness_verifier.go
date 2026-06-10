package compliance

import (
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
	"github.com/AlHarisTech/ai-workstation-core/runtime/queue"
	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

func VerifyFairness(t *testing.T) FairnessReport {
	t.Helper()
	report := FairnessReport{
		Timestamp:     time.Now(),
		SessionStats:  make(map[string]int),
		CompliancePass: true,
	}

	var violations int
	var maxStarvation float64
	var totalWait float64

	violationCb := func(fv queue.FairnessViolation) {
		report.FairnessEvents++
		if fv.Type == "FAIRNESS_VIOLATION" {
			violations++
		}
		if fv.WaitTimeMs > maxStarvation {
			maxStarvation = fv.WaitTimeMs
		}
		totalWait += fv.WaitTimeMs
	}

	fq := queue.NewFairQueue(256)
	fq.SetViolationCallback(violationCb)

	cfg := kernel.DefaultConfig(); ws := tempWorkspaceRoot(); defer os.RemoveAll(ws); cfg.WorkspaceRoot = ws
	cfg.QueueSize = 256
	harden := kernel.DefaultHardeningConfig()

	engine, err := kernel.NewKernelEngine(cfg, harden)
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}

	go func() {
		sessions := map[string]int{"A": 1000, "B": 100, "C": 10}
		for sid, count := range sessions {
			for i := 0; i < count; i++ {
				fq.Enqueue(&types.RequestContext{
					SessionID:      sid,
					TimestampStart: time.Now(),
				})
				report.SessionStats[sid]++
			}
		}
	}()

	time.Sleep(100 * time.Millisecond)
	_ = engine
	fq.Dequeue()

	report.Violations = violations
	report.MaxStarvationMs = maxStarvation
	if report.FairnessEvents > 0 {
		report.AverageWaitMs = totalWait / float64(report.FairnessEvents)
	}

	if violations > 3 || maxStarvation > 30000 {
		report.CompliancePass = false
	}
	return report
}
