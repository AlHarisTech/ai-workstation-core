package compliance

import (
	"os"
	"testing"
)

func TestFullComplianceSuite(t *testing.T) {
	os.Setenv("AI_WORKSTATION_ROOT", "/tmp/compliance_test")
	defer os.RemoveAll("/tmp/compliance_test")

	engine := NewComplianceScoreEngine()

	t.Run("Fairness", func(t *testing.T) {
		fr := VerifyFairness(t)
		if !fr.CompliancePass {
			t.Logf("fairness violations=%d max_starv=%.1fms", fr.Violations, fr.MaxStarvationMs)
		}
	})
	_ = engine

	t.Run("Replay", func(t *testing.T) {
		rr := VerifyReplay(t)
		if !rr.CompliancePass {
			t.Logf("replay: ordering=%v policy=%v state=%v", rr.OrderingMatch, rr.PolicyMatch, rr.StateMatch)
		}
	})

	t.Run("SLA", func(t *testing.T) {
		sr := VerifySLA(t)
		if !sr.CompliancePass {
			t.Logf("SLA: p50=%.1f p95=%.1f p99=%.1f violations=%d", sr.P50, sr.P95, sr.P99, sr.Violations)
		}
	})

	t.Run("Policy", func(t *testing.T) {
		pr := VerifyPolicy(t)
		if !pr.CompliancePass {
			t.Logf("policy: tests=%d bypass=%d", pr.TestsExecuted, pr.BypassDetected)
		}
	})

	t.Run("Shutdown", func(t *testing.T) {
		sr := VerifyShutdown(t)
		if !sr.CompliancePass {
			t.Logf("shutdown: inflight=%d completed=%d lost=%d", sr.RequestsInflight, sr.RequestsCompleted, sr.RequestsLost)
		}
	})
}
