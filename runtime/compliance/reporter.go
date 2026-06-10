package compliance

import (
	"encoding/json"
	"fmt"
	"time"
)

type ComplianceScoreEngine struct{}

func NewComplianceScoreEngine() *ComplianceScoreEngine {
	return &ComplianceScoreEngine{}
}

func (cse *ComplianceScoreEngine) Compute(f *FairnessReport, r *ReplayComplianceReport, s *SLAComplianceReport, p *PolicyComplianceReport, sh *ShutdownComplianceReport) KernelComplianceScore {
	fScore := 0
	if f.CompliancePass {
		fScore = WeightFairness
	}

	rScore := 0
	if r.CompliancePass {
		rScore = WeightReplay
	}

	sScore := 0
	if s.CompliancePass {
		sScore = WeightSLA
	}

	pScore := 0
	if p.CompliancePass {
		pScore = WeightPolicy
	}

	shScore := 0
	if sh.CompliancePass {
		shScore = WeightShutdown
	}

	total := fScore + rScore + sScore + pScore + shScore

	return KernelComplianceScore{
		Version:       "0.6.1",
		TotalScore:    total,
		Level:         CertificationLevel(total),
		FairnessScore: fScore,
		ReplayScore:   rScore,
		SLAScore:      sScore,
		PolicyScore:   pScore,
		ShutdownScore: shScore,
		DomainResults: map[string]string{
			"fairness":  string(PassBool(f.CompliancePass)),
			"replay":    string(PassBool(r.CompliancePass)),
			"sla":       string(PassBool(s.CompliancePass)),
			"policy":    string(PassBool(p.CompliancePass)),
			"shutdown":  string(PassBool(sh.CompliancePass)),
		},
		Timestamp: time.Now(),
	}
}

func (cse *ComplianceScoreEngine) FullReport(f *FairnessReport, r *ReplayComplianceReport, s *SLAComplianceReport, p *PolicyComplianceReport, sh *ShutdownComplianceReport) ComplianceReport {
	return ComplianceReport{
		Score:    cse.Compute(f, r, s, p, sh),
		Fairness: *f,
		Replay:   *r,
		SLA:      *s,
		Policy:   *p,
		Shutdown: *sh,
	}
}

func (cr ComplianceReport) JSON() string {
	data, err := json.MarshalIndent(cr, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":"%s"}`, err.Error())
	}
	return string(data)
}

func (cr ComplianceReport) Summary() string {
	return fmt.Sprintf(
		"Kernel Score: %d/100 [%s]\nFairness: %s | Replay: %s | SLA: %s | Policy: %s | Shutdown: %s",
		cr.Score.TotalScore,
		cr.Score.Level,
		cr.Score.DomainResults["fairness"],
		cr.Score.DomainResults["replay"],
		cr.Score.DomainResults["sla"],
		cr.Score.DomainResults["policy"],
		cr.Score.DomainResults["shutdown"],
	)
}
