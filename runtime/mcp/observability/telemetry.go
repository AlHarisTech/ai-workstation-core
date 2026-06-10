package observability

import (
	"sync"
	"sync/atomic"
)

type TelemetryCollector struct {
	mu             sync.Mutex
	sampleRate     int64
	sampleCount    atomic.Int64

	toolLatency    map[string]*toolLatencyStats
	queueDepth     atomic.Int64
	cbStateCount   map[string]int64
	retryConsumed  atomic.Int64
	saturationHits atomic.Int64
	requestTotal   atomic.Int64
	rejectTotal    atomic.Int64
}

type toolLatencyStats struct {
	mu       sync.Mutex
	buffer   []int64
	pos      int
	count    int
	capacity int
}

func newToolLatencyStats(capacity int) *toolLatencyStats {
	return &toolLatencyStats{
		buffer:   make([]int64, capacity),
		capacity: capacity,
	}
}

func (s *toolLatencyStats) record(ms int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buffer[s.pos] = ms
	s.pos = (s.pos + 1) % s.capacity
	if s.count < s.capacity {
		s.count++
	}
}

func (s *toolLatencyStats) snapshot() (avg int64, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == 0 {
		return 0, 0
	}
	var sum int64
	n := s.count
	for i := 0; i < n; i++ {
		idx := (s.pos - n + i + s.capacity) % s.capacity
		sum += s.buffer[idx]
	}
	return sum / int64(n), n
}

func NewTelemetryCollector(sampleRate int64) *TelemetryCollector {
	if sampleRate <= 0 {
		sampleRate = 1
	}
	return &TelemetryCollector{
		sampleRate:   sampleRate,
		toolLatency:  make(map[string]*toolLatencyStats),
		cbStateCount: make(map[string]int64),
	}
}

func (tc *TelemetryCollector) ShouldSample() bool {
	prev := tc.sampleCount.Add(1)
	return prev%tc.sampleRate == 0
}

func (tc *TelemetryCollector) RecordToolLatency(tool string, ms int64) {
	tc.mu.Lock()
	stats, ok := tc.toolLatency[tool]
	if !ok {
		stats = newToolLatencyStats(100)
		tc.toolLatency[tool] = stats
	}
	tc.mu.Unlock()
	stats.record(ms)
}

func (tc *TelemetryCollector) RecordQueueDepth(depth int) {
	tc.queueDepth.Store(int64(depth))
}

func (tc *TelemetryCollector) RecordCBState(tool string, state string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.cbStateCount[tool+"::"+state]++
}

func (tc *TelemetryCollector) RecordRetryConsumed() {
	tc.retryConsumed.Add(1)
}

func (tc *TelemetryCollector) RecordSaturationHit() {
	tc.saturationHits.Add(1)
}

func (tc *TelemetryCollector) RecordRequest() {
	tc.requestTotal.Add(1)
}

func (tc *TelemetryCollector) RecordReject() {
	tc.rejectTotal.Add(1)
}

type TelemetrySnapshot struct {
	RequestTotal   int64              `json:"request_total"`
	RejectTotal    int64              `json:"reject_total"`
	SaturationHits int64              `json:"saturation_hits"`
	RetryConsumed  int64              `json:"retry_consumed"`
	QueueDepth     int64              `json:"queue_depth"`
	ToolLatency    map[string]int64   `json:"tool_latency_avg_ms"`
	CBStateDist    map[string]int64   `json:"cb_state_distribution"`
	RejectRate     float64            `json:"reject_rate"`
}

func (tc *TelemetryCollector) Snapshot() TelemetrySnapshot {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	total := tc.requestTotal.Load()
	rejects := tc.rejectTotal.Load()

	toolLat := make(map[string]int64)
	for tool, stats := range tc.toolLatency {
		avg, _ := stats.snapshot()
		toolLat[tool] = avg
	}

	cbDist := make(map[string]int64)
	for k, v := range tc.cbStateCount {
		cbDist[k] = v
	}

	rejectRate := 0.0
	if total > 0 {
		rejectRate = float64(rejects) / float64(total)
	}

	return TelemetrySnapshot{
		RequestTotal:   total,
		RejectTotal:    rejects,
		SaturationHits: tc.saturationHits.Load(),
		RetryConsumed:  tc.retryConsumed.Load(),
		QueueDepth:     tc.queueDepth.Load(),
		ToolLatency:    toolLat,
		CBStateDist:    cbDist,
		RejectRate:     rejectRate,
	}
}
