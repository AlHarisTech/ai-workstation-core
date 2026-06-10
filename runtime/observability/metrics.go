package observability

import (
	"sync/atomic"
	"time"
)

type AtomicDuration struct {
	v atomic.Int64
}

func (d *AtomicDuration) Set(dur time.Duration) { d.v.Store(int64(dur)) }
func (d *AtomicDuration) Load() time.Duration   { return time.Duration(d.v.Load()) }

type KernelMetrics struct {
	Ingressed   atomic.Int64
	Enqueued    atomic.Int64
	Rejected    atomic.Int64
	Completed   atomic.Int64
	Denied      atomic.Int64
	Failed      atomic.Int64
	Cycles      atomic.Int64
	QueueDepth  atomic.Int64
	QueueMax    atomic.Int64
	StartTime   time.Time
}

func NewKernelMetrics() *KernelMetrics {
	return &KernelMetrics{StartTime: time.Now()}
}

func (m *KernelMetrics) ThroughputRPS() float64 {
	elapsed := time.Since(m.StartTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(m.Cycles.Load()) / elapsed
}

func (m *KernelMetrics) PolicyDenialRate() float64 {
	total := float64(m.Completed.Load() + m.Denied.Load())
	if total == 0 {
		return 0
	}
	return float64(m.Denied.Load()) / total
}

func (m *KernelMetrics) Snapshot() map[string]interface{} {
	completed := m.Completed.Load()
	denied := m.Denied.Load()
	total := completed + denied

	return map[string]interface{}{
		"throughput_rps":      m.ThroughputRPS(),
		"queue_depth":         m.QueueDepth.Load(),
		"queue_max":           m.QueueMax.Load(),
		"policy_denial_rate":  m.PolicyDenialRate(),
		"requests_ingressed":  m.Ingressed.Load(),
		"requests_enqueued":   m.Enqueued.Load(),
		"requests_rejected":   m.Rejected.Load(),
		"requests_completed":  completed,
		"requests_denied":     denied,
		"requests_failed":     m.Failed.Load(),
		"total_processed":     total,
		"uptime_seconds":      int(time.Since(m.StartTime).Seconds()),
		"cycles":              m.Cycles.Load(),
	}
}
