package observability

import "sync/atomic"

type Metrics struct {
	RequestsIngressed atomic.Int64
	RequestsEnqueued  atomic.Int64
	RequestsRejected  atomic.Int64
	RequestsCompleted atomic.Int64
	RequestsDenied    atomic.Int64
	ExecutionsFailed  atomic.Int64
	WorkerPanics      atomic.Int64
	QueueDepth        atomic.Int64
}

func NewMetrics() *Metrics {
	return &Metrics{}
}

func (m *Metrics) Snapshot() map[string]int64 {
	return map[string]int64{
		"requests_ingressed": m.RequestsIngressed.Load(),
		"requests_enqueued":  m.RequestsEnqueued.Load(),
		"requests_rejected":  m.RequestsRejected.Load(),
		"requests_completed": m.RequestsCompleted.Load(),
		"requests_denied":    m.RequestsDenied.Load(),
		"executions_failed":  m.ExecutionsFailed.Load(),
		"worker_panics":      m.WorkerPanics.Load(),
		"queue_depth":        m.QueueDepth.Load(),
	}
}
