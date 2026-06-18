package signal

// Classifier derives the canonical Signal State from the VerificationResult
// and optional Drift indicators. It is a PURE FUNCTION (AC-003).
type Classifier struct{}

// Classify determines the signal state from verification results.
func (c *Classifier) Classify(v VerificationResult, drift *Drift, divergences []Divergence) State {
	// Consistency failure is the most critical — lattice broken.
	if v.Consistency == VerdictFail {
		return StateConsistencyFail
	}

	// Structural failure means graph constraints are violated.
	if v.Structural == VerdictFail {
		return StateStructuralFail
	}

	// Replay divergence means determinism is broken.
	if v.Replay == VerdictDivergence {
		return StateReplayDivergence
	}

	// Drift detected at CRITICAL level.
	if drift != nil && drift.Detected && drift.Severity == SeverityCritical {
		return StateDriftDetected
	}

	// Any single domain failure.
	failCount := 0
	for _, verdict := range []Verdict{v.Kernel, v.Journal, v.Trace, v.Replay, v.Structural} {
		if verdict == VerdictFail {
			failCount++
		}
	}
	if failCount > 0 {
		return StatePartialFail
	}

	// Drift at lower severity with divergence evidence.
	if drift != nil && drift.Detected && len(divergences) > 0 {
		return StateDriftDetected
	}

	return StateAllPass
}

// SeverityFromDrift maps divergence classes to severity levels.
func SeverityFromDrift(class DivergenceClass) Severity {
	switch class {
	case DivergenceStructural:
		return SeverityCritical
	case DivergenceSequence:
		return SeverityMedium
	case DivergenceTemporal:
		return SeverityLow
	default:
		return SeverityLow
	}
}

// IsDegrading checks if the current signal is worse than the previous one.
func IsDegrading(prev, curr State) bool {
	order := map[State]int{
		StateAllPass:          0,
		StatePartialFail:      1,
		StateDriftDetected:    2,
		StateReplayDivergence: 3,
		StateStructuralFail:   4,
		StateConsistencyFail:  5,
	}
	return order[curr] > order[prev]
}

// IsRecovering checks if the current signal is better than the previous.
func IsRecovering(prev, curr State) bool {
	return IsDegrading(curr, prev)
}
