// Package predict implements the ADR-0009 Drift Predictor as a pure,
// stateless trend analysis engine. It consumes signal history and produces
// deterministic forecasts (AC-004 — trend-based, not ML).
//
// Boundaries:
//   - Never imports MEK internals
//   - Never produces actions (that's feedback's job)
//   - Never interacts with the Action Bus
//   - Pure function: same (history, horizon, now) → same prediction
package predict

import (
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

// Direction describes the trend direction of signal history.
type Direction int

const (
	Stable    Direction = iota // no significant change
	Improving                  // fewer failures over time
	Degrading                  // more failures over time
)

func (d Direction) String() string {
	switch d {
	case Stable:
		return "STABLE"
	case Improving:
		return "IMPROVING"
	case Degrading:
		return "DEGRADING"
	default:
		return "UNKNOWN"
	}
}

// Factor is a named contributor to the prediction with a weight.
type Factor struct {
	Name   string  `json:"factor"`
	Weight float64 `json:"weight"`
}

// Prediction is a deterministic forecast produced from signal history.
// It does NOT recommend actions — only describes expected future state.
type Prediction struct {
	GeneratedAt time.Time     `json:"generated_at"`
	Horizon     time.Duration `json:"horizon_ms"`

	// Trend
	Direction  Direction `json:"direction"`
	Confidence float64   `json:"confidence"`

	// Predicted state
	State       signal.State `json:"predicted_state"`
	Probability float64      `json:"probability"`
	EarliestAt  *time.Time   `json:"earliest_at,omitempty"`

	// Data
	SampleSize int      `json:"sample_size"`
	Factors    []Factor `json:"factors,omitempty"`
}

// Predict analyzes signal history and forecasts the future state.
// It is a PURE FUNCTION: same (history, horizon, now) → same Prediction.
//
// v1 algorithm (ADR-0009 §5.3):
//   1. Compute rate of change for each signal class
//   2. If structural_fail_rate > 0 → STRUCTURAL_FAIL (probability=1.0)
//   3. If replay_divergence_rate > 0 → REPLAY_DIVERGENCE
//   4. If drift_rate increasing → DRIFT_DETECTED
//   5. Otherwise → ALL_PASS
func Predict(history []signal.Signal, horizon time.Duration, now time.Time) Prediction {
	p := Prediction{
		GeneratedAt: now,
		Horizon:     horizon,
		SampleSize:  len(history),
	}

	if len(history) == 0 {
		p.Direction = Stable
		p.Confidence = 0.0
		p.State = signal.StateAllPass
		p.Probability = 0.5 // insufficient data
		return p
	}

	// Compute rates
	replayRate := computeRate(history, signal.StateReplayDivergence, now)
	structFailRate := computeRate(history, signal.StateStructuralFail, now)
	driftRate := computeRate(history, signal.StateDriftDetected, now)
	partialFailRate := computeRate(history, signal.StatePartialFail, now)
	consistencyFailRate := computeRate(history, signal.StateConsistencyFail, now)

	// Structural fail is deterministic — if it happened once, it will persist
	if structFailRate > 0 {
		p.State = signal.StateStructuralFail
		p.Probability = 1.0
		p.Direction = Degrading
		p.Confidence = 1.0
		p.Factors = append(p.Factors, Factor{"structural_fail_rate", structFailRate})
		return p
	}

	// Consistency fail is deterministic
	if consistencyFailRate > 0 {
		p.State = signal.StateConsistencyFail
		p.Probability = 1.0
		p.Direction = Degrading
		p.Confidence = 1.0
		p.Factors = append(p.Factors, Factor{"consistency_fail_rate", consistencyFailRate})
		return p
	}

	// Replay divergence: probability = min(1.0, rate × horizon_factor)
	if replayRate > 0 {
		horizonFactor := horizon.Hours()
		if horizonFactor < 0.01 {
			horizonFactor = 0.01
		}
		prob := replayRate * horizonFactor
		if prob > 1.0 {
			prob = 1.0
		}
		p.State = signal.StateReplayDivergence
		p.Probability = prob
		p.Direction = directionFromRate(replayRate, history, signal.StateReplayDivergence, now)
		p.Confidence = confidenceFromSample(p.SampleSize)
		p.Factors = append(p.Factors, Factor{"replay_divergence_rate", replayRate})
		if prob >= 0.5 {
			earliest := now.Add(time.Duration(1.0/prob) * time.Second)
			p.EarliestAt = &earliest
		}
		return p
	}

	// Drift detected: probability based on trend
	if driftRate > 0 {
		driftDir := directionFromRate(driftRate, history, signal.StateDriftDetected, now)
		if driftDir == Improving {
			// Drift is decreasing — may resolve to ALL_PASS
			lastIsAllPass := len(history) > 0 && history[len(history)-1].State == signal.StateAllPass
			if lastIsAllPass {
				p.State = signal.StateAllPass
				p.Direction = Improving
				p.Confidence = confidenceFromSample(p.SampleSize)
				p.Probability = 1.0 - (driftRate * horizon.Hours() * 0.1)
				if p.Probability < 0.5 {
					p.Probability = 0.5
				}
				p.Factors = append(p.Factors, Factor{"drift_rate", driftRate})
				return p
			}
		}
		p.State = signal.StateDriftDetected
		p.Direction = driftDir
		p.Confidence = confidenceFromSample(p.SampleSize)

		if driftDir == Degrading {
			p.Probability = min(1.0, driftRate*horizon.Hours()*2)
		} else {
			p.Probability = driftRate * horizon.Hours()
		}
		p.Factors = append(p.Factors, Factor{"drift_rate", driftRate})
		return p
	}

	// Partial failures
	if partialFailRate > 0 {
		p.State = signal.StatePartialFail
		p.Direction = directionFromRate(partialFailRate, history, signal.StatePartialFail, now)
		p.Confidence = confidenceFromSample(p.SampleSize)
		p.Probability = partialFailRate * horizon.Hours()
		p.Factors = append(p.Factors, Factor{"partial_fail_rate", partialFailRate})
		return p
	}

	// All stable
	p.State = signal.StateAllPass
	p.Probability = 1.0 - max(replayRate, structFailRate, driftRate, partialFailRate, consistencyFailRate)
	p.Direction = Stable
	p.Confidence = confidenceFromSample(p.SampleSize)
	return p
}

// computeRate returns the occurrence rate (events/hour) of a given state.
func computeRate(history []signal.Signal, state signal.State, now time.Time) float64 {
	var count int
	var earliest time.Time

	for _, s := range history {
		if s.State == state {
			count++
			if earliest.IsZero() || s.Timestamp.Before(earliest) {
				earliest = s.Timestamp
			}
		}
	}

	if count == 0 {
		return 0
	}

	window := now.Sub(earliest).Hours()
	if window < 0.01 {
		window = 0.01
	}

	return float64(count) / window
}

// directionFromRate determines trend direction by comparing recent vs older rates.
func directionFromRate(currentRate float64, history []signal.Signal, state signal.State, now time.Time) Direction {
	if len(history) < 2 {
		return Stable
	}

	// Split history: first half vs second half
	mid := len(history) / 2
	recent := history[mid:]
	old := history[:mid]

	recentRate := computeRate(recent, state, now)
	oldRate := computeRate(old, state, now)

	delta := recentRate - oldRate
	if delta > 0.05 {
		return Degrading
	}
	if delta < -0.05 {
		return Improving
	}
	return Stable
}

// confidenceFromSample returns a confidence score based on sample size.
func confidenceFromSample(n int) float64 {
	if n <= 1 {
		return 0.1
	}
	if n >= 100 {
		return 1.0
	}
	// Logarithmic scale: confidence grows with sample size
	return min(1.0, float64(n)/100.0)
}

func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func max(vals ...float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	m := vals[0]
	for _, v := range vals[1:] {
		if v > m {
			m = v
		}
	}
	return m
}
