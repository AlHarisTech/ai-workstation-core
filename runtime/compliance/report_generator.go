package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func RunFullComplianceSuite(t *testing.T) ComplianceReport {
	t.Helper()

	engine := NewComplianceScoreEngine()

	fr := VerifyFairness(t)
	rr := VerifyReplay(t)
	sr := VerifySLA(t)
	pr := VerifyPolicy(t)
	shr := VerifyShutdown(t)

	report := engine.FullReport(&fr, &rr, &sr, &pr, &shr)
	return report
}

func WriteComplianceReport(report ComplianceReport, outputPath string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0644)
}

func PrintComplianceSummary(report ComplianceReport) {
	fmt.Println("========================================")
	fmt.Println("  MCP Kernel v0.6.1 — Compliance Report")
	fmt.Println("========================================")
	fmt.Printf("  Score: %d / %d  [%s]\n",
		report.Score.TotalScore, MaxScore, report.Score.Level)
	fmt.Println("----------------------------------------")
	fmt.Printf("  Fairness  (%d/%d): %s  violations=%d\n",
		report.Score.FairnessScore, WeightFairness,
		report.Score.DomainResults["fairness"],
		report.Fairness.Violations)
	fmt.Printf("  Replay    (%d/%d): %s  state=%v order=%v\n",
		report.Score.ReplayScore, WeightReplay,
		report.Score.DomainResults["replay"],
		report.Replay.StateMatch, report.Replay.OrderingMatch)
	fmt.Printf("  SLA       (%d/%d): %s  p50=%.1f p95=%.1f p99=%.1f\n",
		report.Score.SLAScore, WeightSLA,
		report.Score.DomainResults["sla"],
		report.SLA.P50, report.SLA.P95, report.SLA.P99)
	fmt.Printf("  Policy    (%d/%d): %s  tests=%d bypass=%d\n",
		report.Score.PolicyScore, WeightPolicy,
		report.Score.DomainResults["policy"],
		report.Policy.TestsExecuted, report.Policy.BypassDetected)
	fmt.Printf("  Shutdown  (%d/%d): %s  lost=%d inflight=%d\n",
		report.Score.ShutdownScore, WeightShutdown,
		report.Score.DomainResults["shutdown"],
		report.Shutdown.RequestsLost, report.Shutdown.RequestsInflight)
	fmt.Println("========================================")
}
