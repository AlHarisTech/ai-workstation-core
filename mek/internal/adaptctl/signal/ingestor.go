package signal

import (
	"fmt"

	"github.com/anomalyco/mek/internal/replay"
	"github.com/anomalyco/mek/internal/verify"
	"github.com/anomalyco/mek/pkg/types"
)

// Ingestor consumes MEK verification outputs and produces canonical Signals.
// It is the ONLY bridge between MEK output and ADR-0009 signal space.
// Boundary: reads MEK output structs, never imports MEK internals beyond public types.
type Ingestor struct {
	classifier *Classifier
}

func NewIngestor() *Ingestor {
	return &Ingestor{classifier: &Classifier{}}
}

// FromStatusMap creates a Signal from a raw MEK StatusMap (kernel-only check).
func (i *Ingestor) FromStatusMap(executionID, projectRef, rirHash string, sm map[string]*types.NodeState) *Signal {
	s := New(executionID, projectRef, rirHash)

	allSuccess := true
	for _, state := range sm {
		s.Metrics.NodesExecuted++
		if state.Status != types.StatusSuccess {
			allSuccess = false
			s.Metrics.NodesFailed++
		}
	}

	if allSuccess {
		s.Verification = VerificationResult{
			Kernel: VerdictPass,
		}
	} else {
		s.Verification = VerificationResult{
			Kernel: VerdictFail,
		}
	}

	s.State = i.classifier.Classify(s.Verification, s.Drift, s.Divergences)
	return s
}

// FromReplayReport creates a Signal from a replay verification report.
func (i *Ingestor) FromReplayReport(executionID, projectRef, rirHash string, rp *replay.Report) *Signal {
	s := New(executionID, projectRef, rirHash)

	verdict := VerdictPass
	if !rp.Match {
		verdict = VerdictDivergence
		for _, d := range rp.Divergences {
			s.Divergences = append(s.Divergences, Divergence{
				Domain:   "kernel↔replay",
				NodeID:   d.NodeID,
				Expected: string(d.JournalStatus),
				Actual:   string(d.ReplayStatus),
			})
		}
	}

	s.Verification = VerificationResult{Replay: verdict}
	s.Metrics = Metrics{
		NodesExecuted: rp.ReplayNodes,
	}

	if len(s.Divergences) > 0 {
		s.Drift = &Drift{
			Detected: true,
			Class:    DivergenceStructural,
			Severity: SeverityCritical,
		}
	}

	s.State = i.classifier.Classify(s.Verification, s.Drift, s.Divergences)
	return s
}

// FromStructuralReport creates a Signal from a structural verification report.
func (i *Ingestor) FromStructuralReport(executionID, projectRef, rirHash string, sr *verify.Report) *Signal {
	s := New(executionID, projectRef, rirHash)

	verdict := VerdictPass
	if !sr.Pass {
		verdict = VerdictFail
		for _, v := range sr.Violations {
			s.Divergences = append(s.Divergences, Divergence{
				Domain: "structural",
				NodeID: v.NodeID,
				Expected: "valid",
				Actual:   fmt.Sprintf("%s: %s", v.Rule, v.Message),
			})
		}
	}

	s.Verification = VerificationResult{Structural: verdict}
	s.Metrics = Metrics{
		NodesExecuted: sr.Stats.TerminalNodes,
	}

	if len(s.Divergences) > 0 {
		s.Drift = &Drift{
			Detected: true,
			Class:    DivergenceStructural,
			Severity: SeverityCritical,
		}
	}

	s.State = i.classifier.Classify(s.Verification, s.Drift, s.Divergences)
	return s
}

// FromConsistencyReport creates a Signal from a cross-domain consistency report.
func (i *Ingestor) FromConsistencyReport(executionID, projectRef, rirHash string, cr *verify.ConsistencyReport) *Signal {
	s := New(executionID, projectRef, rirHash)

	verdict := VerdictPass
	domains := make(map[string]Verdict)
	for _, check := range cr.Checks {
		if check.Pass {
			domains[check.Name] = VerdictPass
		} else {
			domains[check.Name] = VerdictFail
			verdict = VerdictFail
			s.Divergences = append(s.Divergences, Divergence{
				Domain: check.Name,
				Actual: check.Detail,
			})
		}
	}

	s.Verification = VerificationResult{
		Consistency: verdict,
		Journal:     domains["journal↔kernel"],
		Trace:       domains["trace↔journal"],
		Replay:      domains["replay↔journal"],
		Structural:  domains["structural"],
	}

	if !cr.Pass {
		s.Drift = &Drift{
			Detected: true,
			Class:    DivergenceStructural,
			Severity: SeverityCritical,
		}
	}

	s.State = i.classifier.Classify(s.Verification, s.Drift, s.Divergences)
	return s
}

// ─── Composite Ingestors ───

// FromFullVerification creates a Signal from a complete MEK verification run.
// This combines status map, replay, and structural checks into one signal.
func (i *Ingestor) FromFullVerification(
	executionID, projectRef, rirHash string,
	sm map[string]*types.NodeState,
	rp *replay.Report,
	sr *verify.Report,
) *Signal {
	s := New(executionID, projectRef, rirHash)

	// Kernel
	allSuccess := true
	for _, state := range sm {
		s.Metrics.NodesExecuted++
		if state.Status != types.StatusSuccess {
			allSuccess = false
			s.Metrics.NodesFailed++
		}
	}
	kernelVerdict := VerdictPass
	if !allSuccess {
		kernelVerdict = VerdictFail
	}

	// Replay
	replayVerdict := VerdictPass
	if !rp.Match {
		replayVerdict = VerdictDivergence
		for _, d := range rp.Divergences {
			s.Divergences = append(s.Divergences, Divergence{
				Domain:   "kernel↔replay",
				NodeID:   d.NodeID,
				Expected: string(d.JournalStatus),
				Actual:   string(d.ReplayStatus),
			})
		}
	}

	// Structural
	structuralVerdict := VerdictPass
	if !sr.Pass {
		structuralVerdict = VerdictFail
		for _, v := range sr.Violations {
			s.Divergences = append(s.Divergences, Divergence{
				Domain: "structural",
				NodeID: v.NodeID,
				Expected: "valid",
				Actual:   fmt.Sprintf("%s: %s", v.Rule, v.Message),
			})
		}
	}

	s.Verification = VerificationResult{
		Kernel:     kernelVerdict,
		Replay:     replayVerdict,
		Structural: structuralVerdict,
	}
	s.Metrics.NodesExecuted = s.Metrics.NodesExecuted
	if rp != nil {
		s.Metrics.NodesExecuted = rp.ReplayNodes
	}

	if len(s.Divergences) > 0 {
		s.Drift = &Drift{
			Detected: true,
			Class:    DivergenceStructural,
			Severity: SeverityCritical,
		}
	}

	s.State = i.classifier.Classify(s.Verification, s.Drift, s.Divergences)
	return s
}
