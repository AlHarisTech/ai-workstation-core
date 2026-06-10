package compliance

import "time"

type ComplianceStatus string

const (
	Pass ComplianceStatus = "PASS"
	Fail ComplianceStatus = "FAIL"
	Skip ComplianceStatus = "SKIP"
)

type FairnessReport struct {
	MaxStarvationMs  float64           `json:"max_starvation_ms"`
	AverageWaitMs    float64           `json:"average_wait_ms"`
	FairnessEvents   int               `json:"fairness_events"`
	Violations       int               `json:"violations"`
	SessionStats     map[string]int    `json:"session_stats"`
	CompliancePass   bool              `json:"compliance_pass"`
	Timestamp        time.Time         `json:"timestamp"`
}

type ReplayComplianceReport struct {
	OriginalHash    string `json:"original_hash"`
	ReplayHash      string `json:"replay_hash"`
	OrderingMatch   bool   `json:"ordering_match"`
	PolicyMatch     bool   `json:"policy_match"`
	StateMatch      bool   `json:"state_match"`
	CompliancePass  bool   `json:"compliance_pass"`
	Timestamp       time.Time `json:"timestamp"`
}

type SLAComplianceReport struct {
	P50          float64 `json:"p50_ms"`
	P95          float64 `json:"p95_ms"`
	P99          float64 `json:"p99_ms"`
	Max          float64 `json:"max_ms"`
	SampleCount  int     `json:"sample_count"`
	Violations   int     `json:"violations"`
	SLAViolation bool    `json:"sla_violation"`
	CompliancePass bool  `json:"compliance_pass"`
	Timestamp    time.Time `json:"timestamp"`
}

type PolicyComplianceReport struct {
	TestsExecuted  int    `json:"tests_executed"`
	DenyEvents     int    `json:"deny_events"`
	BypassDetected int    `json:"bypass_detected"`
	Details        []PolicyTestResult `json:"details"`
	CompliancePass bool   `json:"compliance_pass"`
	Timestamp      time.Time `json:"timestamp"`
}

type PolicyTestResult struct {
	TestName     string `json:"test_name"`
	ExpectedDeny bool   `json:"expected_deny"`
	WasDenied    bool   `json:"was_denied"`
	Bypassed     bool   `json:"bypassed"`
	Error        string `json:"error,omitempty"`
}

type ShutdownComplianceReport struct {
	RequestsInflight   int  `json:"requests_inflight"`
	RequestsCompleted  int  `json:"requests_completed"`
	RequestsLost       int  `json:"requests_lost"`
	StateFlushSuccess  bool `json:"state_flush_success"`
	CompliancePass     bool `json:"compliance_pass"`
	Timestamp          time.Time `json:"timestamp"`
}

type KernelComplianceScore struct {
	Version        string                `json:"version"`
	TotalScore     int                   `json:"total_score"`
	Level          string                `json:"certification_level"`
	FairnessScore  int                   `json:"fairness_score"`
	ReplayScore    int                   `json:"replay_score"`
	SLAScore       int                   `json:"sla_score"`
	PolicyScore    int                   `json:"policy_score"`
	ShutdownScore  int                   `json:"shutdown_score"`
	DomainResults  map[string]string     `json:"domain_results"`
	Timestamp      time.Time             `json:"timestamp"`
}

type ComplianceReport struct {
	Score     KernelComplianceScore   `json:"score"`
	Fairness  FairnessReport          `json:"fairness"`
	Replay    ReplayComplianceReport  `json:"replay"`
	SLA       SLAComplianceReport     `json:"sla"`
	Policy    PolicyComplianceReport  `json:"policy"`
	Shutdown  ShutdownComplianceReport `json:"shutdown"`
}
