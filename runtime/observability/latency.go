package observability

import (
	"math"
	"sort"
	"sync"
)

type LatencyTracker struct {
	mu         sync.Mutex
	samples    []float64
	maxSamples int
	count      int64
	totalMs    float64
	maxMs      float64
}

func NewLatencyTracker(maxSamples int) *LatencyTracker {
	if maxSamples <= 0 {
		maxSamples = 1000
	}
	return &LatencyTracker{
		samples:    make([]float64, 0, maxSamples),
		maxSamples: maxSamples,
	}
}

func (lt *LatencyTracker) Record(durationMs float64) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	lt.count++
	lt.totalMs += durationMs
	if durationMs > lt.maxMs {
		lt.maxMs = durationMs
	}

	if len(lt.samples) >= lt.maxSamples {
		lt.samples = lt.samples[1:]
	}
	lt.samples = append(lt.samples, durationMs)
}

type LatencySLA struct {
	P50       float64 `json:"p50_ms"`
	P95       float64 `json:"p95_ms"`
	P99       float64 `json:"p99_ms"`
	Max       float64 `json:"max_ms"`
	Avg       float64 `json:"avg_ms"`
	Count     int64   `json:"count"`
	Violation bool    `json:"sla_violation"`
}

func (lt *LatencyTracker) Snapshot() LatencySLA {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	count := lt.count
	if count == 0 {
		return LatencySLA{}
	}

	sorted := make([]float64, len(lt.samples))
	copy(sorted, lt.samples)
	sort.Float64s(sorted)

	sla := LatencySLA{
		P50:   percentile(sorted, 50),
		P95:   percentile(sorted, 95),
		P99:   percentile(sorted, 99),
		Max:   lt.maxMs,
		Avg:   math.Round(lt.totalMs/float64(count)*100) / 100,
		Count: count,
	}

	if sla.P95 > 50 || sla.P99 > 120 {
		sla.Violation = true
	}
	return sla
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(float64(len(sorted))*p/100)) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return math.Round(sorted[idx]*100) / 100
}
