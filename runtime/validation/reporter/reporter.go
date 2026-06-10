package reporter

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/AlHarisTech/ai-workstation-core/runtime/validation/contracts"
	vtypes "github.com/AlHarisTech/ai-workstation-core/runtime/validation/types"
)

func ComputeReadiness(ci, race bool, stressResults []vtypes.StressValidationReport, failureResults vtypes.FailureInjectionReport) vtypes.ReleaseReadinessReport {
	ciScore := 0
	ciStatus := "FAIL"
	if ci {
		ciScore = contracts.WeightCI
		ciStatus = "PASS"
	}

	raceScore := 0
	raceStatus := "FAIL"
	if race {
		raceScore = contracts.WeightRace
		raceStatus = "PASS"
	}

	stressScore := 0
	stressStatus := "FAIL"
	if len(stressResults) > 0 {
		allPass := true
		for _, r := range stressResults {
			if !r.ValidationPass {
				allPass = false
			}
		}
		if allPass {
			stressScore = contracts.WeightStress
			stressStatus = "PASS"
		}
	}

	failureScore := 0
	failureStatus := "FAIL"
	if failureResults.ValidationPass {
		failureScore = contracts.WeightFailureInjection
		failureStatus = "PASS"
	}

	total := ciScore + raceScore + stressScore + failureScore

	return vtypes.ReleaseReadinessReport{
		Version:         "0.6.2",
		ReadinessScore:  total,
		Status:          contracts.ReadinessLevel(total),
		CI:              ciStatus,
		Race:            raceStatus,
		Stress:          stressStatus,
		FailureInjection: failureStatus,
		StressResults:   stressResults,
		FailureResults:  failureResults.Scenarios,
	}
}

func WriteReport(report vtypes.ReleaseReadinessReport, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func PrintReport(report vtypes.ReleaseReadinessReport) {
	fmt.Println("==============================================")
	fmt.Println("  MCP Kernel v0.6.2 — Release Readiness")
	fmt.Println("==============================================")
	fmt.Printf("  Score: %d / %d  [%s]\n",
		report.ReadinessScore, contracts.MaxReadinessScore, report.Status)
	fmt.Println("----------------------------------------------")
	fmt.Printf("  CI                (%d/%d): %s\n",
		contracts.WeightCI, contracts.WeightCI, report.CI)
	fmt.Printf("  Race Detection    (%d/%d): %s\n",
		contracts.WeightRace, contracts.WeightRace, report.Race)
	fmt.Printf("  Stress            (%d/%d): %s",
		contracts.WeightStress, contracts.WeightStress, report.Stress)
	if len(report.StressResults) > 0 {
		fmt.Printf(" (%d scenarios)", len(report.StressResults))
	}
	fmt.Println()
	fmt.Printf("  Failure Injection (%d/%d): %s",
		contracts.WeightFailureInjection, contracts.WeightFailureInjection, report.FailureInjection)
	if len(report.FailureResults) > 0 {
		pass := 0
		for _, f := range report.FailureResults {
			if f.Pass {
				pass++
			}
		}
		fmt.Printf(" (%d/%d passed)", pass, len(report.FailureResults))
	}
	fmt.Println()
	fmt.Println("==============================================")
}
