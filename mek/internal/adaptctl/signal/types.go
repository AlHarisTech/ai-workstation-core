// Package signal defines the canonical signal types for ADR-0009.
// Boundary: consumes MEK verification outputs only. Never imports MEK internals.
package signal

import "time"

// State is the classified state of a verification signal.
type State string

const (
	StateAllPass           State = "ALL_PASS"
	StatePartialFail       State = "PARTIAL_FAIL"
	StateReplayDivergence  State = "REPLAY_DIVERGENCE"
	StateStructuralFail    State = "STRUCTURAL_FAIL"
	StateDriftDetected     State = "DRIFT_DETECTED"
	StateConsistencyFail   State = "CONSISTENCY_FAIL"
)

// Domain identifies a verification domain within MEK.
type Domain string

const (
	DomainKernel      Domain = "kernel"
	DomainJournal     Domain = "journal"
	DomainTrace       Domain = "trace"
	DomainReplay      Domain = "replay"
	DomainStructural  Domain = "structural"
	DomainConsistency Domain = "consistency"
)

// Verdict is the result of a single domain check.
type Verdict string

const (
	VerdictPass       Verdict = "PASS"
	VerdictFail       Verdict = "FAIL"
	VerdictDivergence Verdict = "DIVERGENCE"
)

// DivergenceClass categorizes the type of drift detected.
type DivergenceClass string

const (
	DivergenceStructural   DivergenceClass = "STRUCTURAL"
	DivergenceTemporal     DivergenceClass = "TEMPORAL"
	DivergenceSequence     DivergenceClass = "SEQUENCE"
	DivergenceUnspecified  DivergenceClass = "UNSPECIFIED"
)

// Severity is the impact level of a drift or failure.
type Severity string

const (
	SeverityLow      Severity = "LOW"
	SeverityMedium   Severity = "MEDIUM"
	SeverityCritical Severity = "CRITICAL"
)

// ─── Signal ───

// Signal is the canonical representation of a MEK verification result.
// Immutable after creation (AC-008).
type Signal struct {
	ID           string             `json:"signal_id"`
	Timestamp    time.Time          `json:"timestamp"`
	Source       Source             `json:"source"`
	Verification VerificationResult `json:"verification"`
	Divergences  []Divergence       `json:"divergences,omitempty"`
	Metrics      Metrics            `json:"metrics"`
	Drift        *Drift             `json:"drift,omitempty"`
	State        State              `json:"state"` // classified by Classifier
}

// Source identifies the origin of the verification signal.
type Source struct {
	ExecutionID string `json:"execution_id"`
	ProjectRef  string `json:"project_ref"`
	RIRHash     string `json:"rir_hash"`
}

// VerificationResult holds the verdict for each truth domain.
type VerificationResult struct {
	Kernel      Verdict `json:"kernel"`
	Journal     Verdict `json:"journal"`
	Trace       Verdict `json:"trace"`
	Replay      Verdict `json:"replay"`
	Structural  Verdict `json:"structural"`
	Consistency Verdict `json:"consistency"`
}

// Divergence records a mismatch between two domains.
type Divergence struct {
	Domain   string `json:"domain"`
	NodeID   string `json:"node_id"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// Metrics carries execution statistics.
type Metrics struct {
	TotalDurationMs    int  `json:"total_duration_ms"`
	NodesExecuted      int  `json:"nodes_executed"`
	NodesFailed        int  `json:"nodes_failed"`
	WavesCompleted     int  `json:"waves_completed"`
	EscalationRequested bool `json:"escalation_requested"`
}

// Drift carries ECCC drift indicators (ADR-0006).
type Drift struct {
	Detected bool            `json:"detected"`
	Class    DivergenceClass `json:"class"`
	Severity Severity        `json:"severity"`
}

// ─── Constructor ───

// New creates a Signal with the current timestamp and a generated ID.
func New(executionID, projectRef, rirHash string) *Signal {
	return &Signal{
		ID:        executionID + "-" + time.Now().Format("150405"),
		Timestamp: time.Now().UTC(),
		Source: Source{
			ExecutionID: executionID,
			ProjectRef:  projectRef,
			RIRHash:     rirHash,
		},
	}
}

// IsTerminal returns true if the signal state requires immediate action.
func (s *Signal) IsTerminal() bool {
	return s.State == StateStructuralFail || s.State == StateConsistencyFail
}

// IsDegraded returns true if the signal indicates degradation.
func (s *Signal) IsDegraded() bool {
	return s.State == StatePartialFail || s.State == StateReplayDivergence || s.State == StateDriftDetected
}

// IsStable returns true if all domains passed.
func (s *Signal) IsStable() bool {
	return s.State == StateAllPass
}
