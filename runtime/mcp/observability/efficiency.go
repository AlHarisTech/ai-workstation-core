package observability

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LatencyBudget enforces a max processing time per request.
type LatencyBudget struct {
	maxDuration time.Duration
}

func NewLatencyBudget(max time.Duration) *LatencyBudget {
	if max <= 0 {
		max = 30 * time.Second
	}
	return &LatencyBudget{maxDuration: max}
}

func (lb *LatencyBudget) MaxDuration() time.Duration { return lb.maxDuration }

func (lb *LatencyBudget) Check(start time.Time) error {
	if time.Since(start) > lb.maxDuration {
		return fmt.Errorf("LATENCY_BUDGET_EXCEEDED: %v", lb.maxDuration)
	}
	return nil
}

// FastPathConfig controls when non-essential instrumentation is skipped.
type FastPathConfig struct {
	skipThreshold float64
	traceEnabled  bool
}

func DefaultFastPathConfig() FastPathConfig {
	return FastPathConfig{
		skipThreshold: 0.3,
		traceEnabled:  true,
	}
}

func (fc FastPathConfig) ShouldSkipTrace(loadPct float64) bool {
	return !fc.traceEnabled || loadPct < fc.skipThreshold
}

// SessionTracker enforces session lifecycle for long-run stability.
type SessionTracker struct {
	mu          sync.Mutex
	sessions    map[string]time.Time
	maxIdle     time.Duration
	maxSessions int
}

func NewSessionTracker(maxSessions int, maxIdle time.Duration) *SessionTracker {
	if maxSessions <= 0 {
		maxSessions = 100
	}
	if maxIdle <= 0 {
		maxIdle = 30 * time.Minute
	}
	return &SessionTracker{
		sessions:    make(map[string]time.Time),
		maxIdle:     maxIdle,
		maxSessions: maxSessions,
	}
}

func (st *SessionTracker) Touch(sessionID string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.sessions[sessionID] = time.Now()
}

func (st *SessionTracker) IsStale(sessionID string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	last, ok := st.sessions[sessionID]
	return !ok || time.Since(last) > st.maxIdle
}

func (st *SessionTracker) EvictStale() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	now := time.Now()
	evicted := 0
	for id, last := range st.sessions {
		if now.Sub(last) > st.maxIdle {
			delete(st.sessions, id)
			evicted++
		}
	}
	return evicted
}

func (st *SessionTracker) ActiveCount() int {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.sessions)
}

func (st *SessionTracker) CanAccept() bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return len(st.sessions) < st.maxSessions
}

// KernelSignalReader reads kernel signals without modifying kernel.
type KernelSignalReader struct {
	saturationFn func() float64
	healthFn     func() float64
}

func NewKernelSignalReader(saturationFn func() float64, healthFn func() float64) *KernelSignalReader {
	return &KernelSignalReader{
		saturationFn: saturationFn,
		healthFn:     healthFn,
	}
}

func (r *KernelSignalReader) SaturationPct() float64 {
	if r.saturationFn == nil {
		return 0
	}
	return r.saturationFn()
}

func (r *KernelSignalReader) HealthScore() float64 {
	if r.healthFn == nil {
		return 1.0
	}
	return r.healthFn()
}

var _ = context.Background
var _ = fmt.Sprintf
