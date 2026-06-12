package metrics

import (
	"encoding/json"
	"sync"
)

const (
	MaxTraceFieldSize  = 512
	MaxTraceTotalSize  = 64 * 1024
	MaxAggregatedTraces = 1000
)

type AggregatedTrace struct {
	TraceID        string            `json:"trace_id"`
	RequestID      string            `json:"request_id"`
	SelectedServer string            `json:"selected_server"`
	StageCount     int               `json:"stage_count"`
	Stages         []string          `json:"stages"`
	TotalLatencyNs int64             `json:"total_latency_ns"`
}

type StageLatencyRecord struct {
	Stage       string `json:"stage"`
	Count       int64  `json:"count"`
	TotalNs     int64  `json:"total_ns"`
	MinNs       int64  `json:"min_ns"`
	MaxNs       int64  `json:"max_ns"`
}

type TraceAggregator struct {
	mu       sync.RWMutex
	traces   []AggregatedTrace
	stageLat map[string]*StageLatencyRecord
	totalBytes int64
}

func NewTraceAggregator() *TraceAggregator {
	return &TraceAggregator{
		traces:   make([]AggregatedTrace, 0, MaxAggregatedTraces),
		stageLat: make(map[string]*StageLatencyRecord),
	}
}

func (ta *TraceAggregator) RecordTrace(traceID, requestID, selectedServer string, stages []string, latencyNs int64) {
	at := AggregatedTrace{
		TraceID:        truncate(traceID, MaxTraceFieldSize),
		RequestID:      truncate(requestID, MaxTraceFieldSize),
		SelectedServer: truncate(selectedServer, MaxTraceFieldSize),
		StageCount:     len(stages),
		Stages:         stages,
		TotalLatencyNs: latencyNs,
	}

	data, _ := json.Marshal(at)
	if ta.totalBytes+int64(len(data)) > MaxTraceTotalSize {
		return
	}

	ta.mu.Lock()
	defer ta.mu.Unlock()

	if len(ta.traces) >= MaxAggregatedTraces {
		ta.traces = ta.traces[1:]
	}
	ta.traces = append(ta.traces, at)
	ta.totalBytes += int64(len(data))
}

func (ta *TraceAggregator) RecordStageLatency(stage string, latencyNs int64) {
	ta.mu.Lock()
	defer ta.mu.Unlock()

	r, ok := ta.stageLat[stage]
	if !ok {
		r = &StageLatencyRecord{Stage: stage, MinNs: latencyNs}
		ta.stageLat[stage] = r
	}
	r.Count++
	r.TotalNs += latencyNs
	if latencyNs < r.MinNs {
		r.MinNs = latencyNs
	}
	if latencyNs > r.MaxNs {
		r.MaxNs = latencyNs
	}
}

func (ta *TraceAggregator) Traces() []AggregatedTrace {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	out := make([]AggregatedTrace, len(ta.traces))
	copy(out, ta.traces)
	return out
}

func (ta *TraceAggregator) StageLatencyStats() []StageLatencyRecord {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	out := make([]StageLatencyRecord, 0, len(ta.stageLat))
	for _, r := range ta.stageLat {
		out = append(out, *r)
	}
	return out
}

func (ta *TraceAggregator) TraceCount() int {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	return len(ta.traces)
}

func (ta *TraceAggregator) ByteUsage() int64 {
	ta.mu.RLock()
	defer ta.mu.RUnlock()
	return ta.totalBytes
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}
