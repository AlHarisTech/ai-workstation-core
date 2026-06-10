package stress

import (
	"testing"
)

func TestStressScenarios(t *testing.T) {
	results := RunStressTests(t)
	for _, r := range results {
		t.Logf("[%s] %d req / %d sessions: failures=%d pass=%v",
			r.Scenario, r.Requests, r.Sessions, r.Failures, r.ValidationPass)
		if !r.ValidationPass {
			t.Errorf("[%s] validation failed: failures=%d", r.Scenario, r.Failures)
		}
	}
}
