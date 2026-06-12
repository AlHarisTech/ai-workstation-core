package metrics

import (
	"encoding/json"
	"sync"
)

var globalRegistry *MetricsRegistry
var registryMu sync.Mutex

type MetricsSnapshot struct {
	Runtime     RuntimeSnapshot     `json:"runtime"`
	Gateway     GatewaySnapshot     `json:"gateway"`
	Enforcement EnforcementSnapshot `json:"enforcement"`
	PolicyIntel PolicyIntelSnapshot `json:"policy_intel"`
	Learning    LearningSnapshot    `json:"learning"`
	Stages      []StageSnapshot     `json:"stages"`
}

type RuntimeSnapshot struct {
	UptimeSeconds int64   `json:"uptime_seconds"`
	ThroughputRPS float64 `json:"throughput_rps"`
	BlockRate     float64 `json:"block_rate"`
	ErrorRate     float64 `json:"error_rate"`
}

type GatewaySnapshot struct {
	RequestsTotal   int64 `json:"requests_total"`
	RequestsAllowed int64 `json:"requests_allowed"`
	RequestsBlocked int64 `json:"requests_blocked"`
	RequestsFailed  int64 `json:"requests_failed"`
	RateLimited     int64 `json:"rate_limited"`
	Failures        int64 `json:"failures"`
	Panics          int64 `json:"panics"`
}

type EnforcementSnapshot struct {
	Evaluations int64 `json:"evaluations"`
	Allowed     int64 `json:"allowed"`
	Blocked     int64 `json:"blocked"`
	Violations  int64 `json:"violations"`
	ErrorCount  int64 `json:"error_count"`
}

type PolicyIntelSnapshot struct {
	EventsRecorded  int64 `json:"events_recorded"`
	DriftDetections int64 `json:"drift_detections"`
	Suggestions     int64 `json:"suggestions"`
	WeightUpdates   int64 `json:"weight_updates"`
}

type LearningSnapshot struct {
	Updates       int64 `json:"updates"`
	SuccessCount  int64 `json:"success_count"`
	FailureCount  int64 `json:"failure_count"`
	AvgLatencyMs  int64 `json:"avg_latency_ms"`
}

type StageSnapshot struct {
	Label        string  `json:"label"`
	Invocations  int64   `json:"invocations"`
	AvgLatencyNs int64   `json:"avg_latency_ns"`
	MaxLatencyNs int64   `json:"max_latency_ns"`
	Failures     int64   `json:"failures"`
}

type MetricsRegistry struct {
	Runtime *RuntimeMetrics
}

func NewMetricsRegistry() *MetricsRegistry {
	return &MetricsRegistry{
		Runtime: NewRuntimeMetrics(),
	}
}

func Global() *MetricsRegistry {
	registryMu.Lock()
	defer registryMu.Unlock()
	if globalRegistry == nil {
		globalRegistry = NewMetricsRegistry()
	}
	return globalRegistry
}

func ResetGlobal() {
	registryMu.Lock()
	defer registryMu.Unlock()
	globalRegistry = NewMetricsRegistry()
}

func (mr *MetricsRegistry) RecordRequest(allowed bool) {
	mr.Runtime.Gateway.RequestsTotal.Add(1)
	if allowed {
		mr.Runtime.Gateway.RequestsAllowed.Add(1)
	} else {
		mr.Runtime.Gateway.RequestsBlocked.Add(1)
	}
}

func (mr *MetricsRegistry) RecordFailure() {
	mr.Runtime.Gateway.RequestsFailed.Add(1)
}

func (mr *MetricsRegistry) RecordRateLimit() {
	mr.Runtime.Gateway.RateLimited.Add(1)
}

func (mr *MetricsRegistry) RecordPanic() {
	mr.Runtime.Gateway.Panics.Add(1)
}

func (mr *MetricsRegistry) RecordEnforcement(allowed bool) {
	mr.Runtime.Enforcement.Evaluations.Add(1)
	if allowed {
		mr.Runtime.Enforcement.Allowed.Add(1)
	} else {
		mr.Runtime.Enforcement.Blocked.Add(1)
	}
}

func (mr *MetricsRegistry) RecordViolation() {
	mr.Runtime.Enforcement.Violations.Add(1)
}

func (mr *MetricsRegistry) RecordPolicyEvent() {
	mr.Runtime.PolicyIntel.EventsRecorded.Add(1)
}

func (mr *MetricsRegistry) RecordDriftDetection() {
	mr.Runtime.PolicyIntel.DriftDetections.Add(1)
}

func (mr *MetricsRegistry) RecordSuggestion() {
	mr.Runtime.PolicyIntel.Suggestions.Add(1)
}

func (mr *MetricsRegistry) RecordWeightUpdate() {
	mr.Runtime.PolicyIntel.WeightUpdates.Add(1)
}

func (mr *MetricsRegistry) RecordLearningUpdate(success bool, latencyMs int64) {
	mr.Runtime.Learning.Updates.Add(1)
	mr.Runtime.Learning.TotalLatencyMs.Add(latencyMs)
	if success {
		mr.Runtime.Learning.SuccessCount.Add(1)
	} else {
		mr.Runtime.Learning.FailureCount.Add(1)
	}
}

func (mr *MetricsRegistry) Snapshot() MetricsSnapshot {
	rm := mr.Runtime
	snap := MetricsSnapshot{
		Runtime: RuntimeSnapshot{
			UptimeSeconds: int64(rm.Uptime().Seconds()),
			ThroughputRPS: rm.ThroughputRPS(),
			BlockRate:     rm.BlockRate(),
			ErrorRate:     rm.ErrorRate(),
		},
		Gateway: GatewaySnapshot{
			RequestsTotal:   rm.Gateway.RequestsTotal.Load(),
			RequestsAllowed: rm.Gateway.RequestsAllowed.Load(),
			RequestsBlocked: rm.Gateway.RequestsBlocked.Load(),
			RequestsFailed:  rm.Gateway.RequestsFailed.Load(),
			RateLimited:     rm.Gateway.RateLimited.Load(),
			Failures:        rm.Gateway.Failures.Load(),
			Panics:          rm.Gateway.Panics.Load(),
		},
		Enforcement: EnforcementSnapshot{
			Evaluations: rm.Enforcement.Evaluations.Load(),
			Allowed:     rm.Enforcement.Allowed.Load(),
			Blocked:     rm.Enforcement.Blocked.Load(),
			Violations:  rm.Enforcement.Violations.Load(),
			ErrorCount:  rm.Enforcement.ErrorCount.Load(),
		},
		PolicyIntel: PolicyIntelSnapshot{
			EventsRecorded:  rm.PolicyIntel.EventsRecorded.Load(),
			DriftDetections: rm.PolicyIntel.DriftDetections.Load(),
			Suggestions:     rm.PolicyIntel.Suggestions.Load(),
			WeightUpdates:   rm.PolicyIntel.WeightUpdates.Load(),
		},
		Learning: LearningSnapshot{
			Updates:      rm.Learning.Updates.Load(),
			SuccessCount: rm.Learning.SuccessCount.Load(),
			FailureCount: rm.Learning.FailureCount.Load(),
		},
	}

	updates := rm.Learning.Updates.Load()
	if updates > 0 {
		snap.Learning.AvgLatencyMs = rm.Learning.TotalLatencyMs.Load() / updates
	}

	for _, s := range rm.Stages {
		inv := s.Invocations.Load()
		avg := int64(0)
		if inv > 0 {
			avg = s.TotalLatency.Load() / inv
		}
		snap.Stages = append(snap.Stages, StageSnapshot{
			Label:        s.Label,
			Invocations:  inv,
			AvgLatencyNs: avg,
			MaxLatencyNs: s.MaxLatency.Load(),
			Failures:     s.Failures.Load(),
		})
	}

	if snap.Stages == nil {
		snap.Stages = []StageSnapshot{}
	}

	return snap
}

func (mr *MetricsRegistry) SnapshotJSON() ([]byte, error) {
	return json.MarshalIndent(mr.Snapshot(), "", "  ")
}
