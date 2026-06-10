package types

import "time"

type CIValidationReport struct {
	TestsPassed      int  `json:"tests_passed"`
	TestsFailed      int  `json:"tests_failed"`
	CompliancePassed bool `json:"compliance_passed"`
	BuildPassed      bool `json:"build_passed"`
	CertificationPass bool `json:"certification_pass"`
	Timestamp        time.Time `json:"timestamp"`
}

type RaceValidationReport struct {
	PackagesTested int  `json:"packages_tested"`
	RacesDetected  int  `json:"races_detected"`
	Failures       int  `json:"failures"`
	ValidationPass bool `json:"validation_pass"`
	Timestamp      time.Time `json:"timestamp"`
}

type StressScenario struct {
	Name     string `json:"name"`
	Sessions int    `json:"sessions"`
	Requests int    `json:"requests"`
}

type StressValidationReport struct {
	Scenario       string  `json:"scenario"`
	Requests       int     `json:"requests"`
	Sessions       int     `json:"sessions"`
	P50            float64 `json:"p50_ms"`
	P95            float64 `json:"p95_ms"`
	P99            float64 `json:"p99_ms"`
	Failures       int     `json:"failures"`
	ValidationPass bool    `json:"validation_pass"`
	Timestamp      time.Time `json:"timestamp"`
}

type FailureScenario struct {
	Name             string `json:"name"`
	ExpectedBehavior string `json:"expected_behavior"`
	ObservedBehavior string `json:"observed_behavior"`
	Pass             bool   `json:"pass"`
}

type FailureInjectionReport struct {
	Scenarios    []FailureScenario `json:"scenarios"`
	TotalPass    int               `json:"total_pass"`
	TotalFail    int               `json:"total_fail"`
	ValidationPass bool            `json:"validation_pass"`
	Timestamp    time.Time         `json:"timestamp"`
}

type ReleaseReadinessReport struct {
	Version        string                `json:"version"`
	ReadinessScore int                   `json:"readiness_score"`
	Status         string                `json:"status"`
	CI             string                `json:"ci"`
	Race           string                `json:"race"`
	Stress         string                `json:"stress"`
	FailureInjection string              `json:"failure_injection"`
	StressResults  []StressValidationReport    `json:"stress_results"`
	FailureResults []FailureScenario           `json:"failure_results"`
	Timestamp      time.Time             `json:"timestamp"`
}
