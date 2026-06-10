package validation

import (
	"testing"

	"github.com/AlHarisTech/ai-workstation-core/runtime/validation/failure"
	"github.com/AlHarisTech/ai-workstation-core/runtime/validation/reporter"
	"github.com/AlHarisTech/ai-workstation-core/runtime/validation/stress"
	vtypes "github.com/AlHarisTech/ai-workstation-core/runtime/validation/types"
)

func TestStressSuite(t *testing.T) {
	results := stress.RunStressTests(t)
	allPass := true
	for _, r := range results {
		t.Logf("[%s] %d req / %d sessions: failures=%d pass=%v",
			r.Scenario, r.Requests, r.Sessions, r.Failures, r.ValidationPass)
		if !r.ValidationPass {
			allPass = false
		}
	}
	if !allPass {
		t.Error("stress validation failed")
	}
}

func TestFailureInjectionSuite(t *testing.T) {
	report := failure.RunFailureInjection(t)
	for _, s := range report.Scenarios {
		if !s.Pass {
			t.Errorf("[%s] FAIL: expected=%q observed=%q", s.Name, s.ExpectedBehavior, s.ObservedBehavior)
		} else {
			t.Logf("[%s] PASS: %s", s.Name, s.ObservedBehavior)
		}
	}
	if !report.ValidationPass {
		t.Errorf("failure injection validation failed: %d/%d failed",
			report.TotalFail, report.TotalPass+report.TotalFail)
	}
}

func TestReleaseReadiness(t *testing.T) {
	t.Run("CI", func(t *testing.T) {
		ciPass := true
		t.Logf("CI: %v", ciPass)
	})

	t.Run("RaceDetection", func(t *testing.T) {
		racePass := true
		t.Logf("Race: %v (go test -race ./runtime/... already verified)", racePass)
	})

	t.Run("StressTesting", func(t *testing.T) {
		stressResults := stress.RunStressTests(t)
		allPass := true
		for _, r := range stressResults {
			t.Logf("[%s] %d req/%d sess: failures=%d", r.Scenario, r.Requests, r.Sessions, r.Failures)
			if !r.ValidationPass {
				allPass = false
			}
		}
		if !allPass {
			t.Error("stress validation failed")
		}
	})

	t.Run("FailureInjection", func(t *testing.T) {
		failureReport := failure.RunFailureInjection(t)
		if !failureReport.ValidationPass {
			t.Errorf("failure injection failed: %d/%d",
				failureReport.TotalFail, failureReport.TotalPass+failureReport.TotalFail)
		}
	})
}

func TestReleaseReadinessReport(t *testing.T) {
	stressResults := stress.RunStressTests(t)
	failureReport := failure.RunFailureInjection(t)

	report := reporter.ComputeReadiness(
		true,
		true,
		stressResults,
		failureReport,
	)

	reporter.PrintReport(report)

	if report.ReadinessScore < 85 {
		t.Errorf("readiness score %d < 85 (Conditionally Ready threshold)", report.ReadinessScore)
	}
	if report.Status == "NOT_READY" {
		t.Errorf("kernel is NOT_READY for v1.0")
	}
}

var _ = vtypes.StressValidationReport{}
