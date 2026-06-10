package failure

import (
	"testing"
)

func TestFailureInjectionScenarios(t *testing.T) {
	report := RunFailureInjection(t)
	for _, s := range report.Scenarios {
		if !s.Pass {
			t.Errorf("[%s] FAIL: expected=%q observed=%q", s.Name, s.ExpectedBehavior, s.ObservedBehavior)
		} else {
			t.Logf("[%s] PASS: %s", s.Name, s.ObservedBehavior)
		}
	}
	if !report.ValidationPass {
		t.Errorf("failure injection validation failed: %d/%d total", report.TotalFail, report.TotalPass)
	}
}
