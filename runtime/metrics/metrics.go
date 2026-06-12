package metrics

import (
	"sync/atomic"
	"time"
)

type GatewayMetrics struct {
	RequestsTotal    atomic.Int64
	RequestsAllowed  atomic.Int64
	RequestsBlocked  atomic.Int64
	RequestsFailed   atomic.Int64
	RateLimited      atomic.Int64
	Failures         atomic.Int64
	Panics           atomic.Int64
	StartTime        time.Time
}

type StageMetrics struct {
	Label        string
	Invocations  atomic.Int64
	TotalLatency atomic.Int64
	MaxLatency   atomic.Int64
	Failures     atomic.Int64
}

type EnforcementMetrics struct {
	Evaluations  atomic.Int64
	Allowed      atomic.Int64
	Blocked      atomic.Int64
	Violations   atomic.Int64
	ErrorCount   atomic.Int64
}

type PolicyIntelMetrics struct {
	EventsRecorded  atomic.Int64
	DriftDetections atomic.Int64
	Suggestions     atomic.Int64
	WeightUpdates   atomic.Int64
}

type LearningMetrics struct {
	Updates       atomic.Int64
	SuccessCount  atomic.Int64
	FailureCount  atomic.Int64
	TotalLatencyMs atomic.Int64
}

type RuntimeMetrics struct {
	Gateway   GatewayMetrics
	Enforcement EnforcementMetrics
	PolicyIntel PolicyIntelMetrics
	Learning  LearningMetrics
	Stages    map[string]*StageMetrics
	mu        chan struct{}
}

func NewRuntimeMetrics() *RuntimeMetrics {
	return &RuntimeMetrics{
		Gateway: GatewayMetrics{StartTime: time.Now()},
		Stages:  make(map[string]*StageMetrics),
		mu:      make(chan struct{}, 1),
	}
}

func (rm *RuntimeMetrics) Stage(name string) *StageMetrics {
	rm.mu <- struct{}{}
	defer func() { <-rm.mu }()
	if s, ok := rm.Stages[name]; ok {
		return s
	}
	s := &StageMetrics{Label: name}
	rm.Stages[name] = s
	return s
}

func (rm *RuntimeMetrics) Uptime() time.Duration {
	return time.Since(rm.Gateway.StartTime)
}

func (rm *RuntimeMetrics) ThroughputRPS() float64 {
	elapsed := time.Since(rm.Gateway.StartTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(rm.Gateway.RequestsTotal.Load()) / elapsed
}

func (rm *RuntimeMetrics) BlockRate() float64 {
	total := rm.Gateway.RequestsTotal.Load()
	if total == 0 {
		return 0
	}
	return float64(rm.Gateway.RequestsBlocked.Load()) / float64(total)
}

func (rm *RuntimeMetrics) ErrorRate() float64 {
	total := rm.Gateway.RequestsTotal.Load()
	if total == 0 {
		return 0
	}
	return float64(rm.Gateway.RequestsFailed.Load()) / float64(total)
}
