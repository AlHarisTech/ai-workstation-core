package observability

import (
	"context"
	"sync"
	"time"
)

const maxTraceEvents = 8

type traceGraphKey struct{}

type TraceGraph struct {
	mu     sync.Mutex
	Events []TraceEvent `json:"events"`
	pool   bool
}

func NewTraceGraph() *TraceGraph {
	return &TraceGraph{
		Events: make([]TraceEvent, 0, maxTraceEvents),
	}
}

func (tg *TraceGraph) Add(evt TraceEvent) {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if len(tg.Events) >= maxTraceEvents {
		copy(tg.Events, tg.Events[1:])
		tg.Events = tg.Events[:maxTraceEvents-1]
	}
	tg.Events = append(tg.Events, evt)
}

func (tg *TraceGraph) EventsCopy() []TraceEvent {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	out := make([]TraceEvent, len(tg.Events))
	copy(out, tg.Events)
	return out
}

func (tg *TraceGraph) Reconstruct() []TraceEvent {
	return tg.EventsCopy()
}

func (tg *TraceGraph) Duration() time.Duration {
	tg.mu.Lock()
	defer tg.mu.Unlock()
	if len(tg.Events) < 2 {
		return 0
	}
	start := time.UnixMilli(tg.Events[0].Timestamp)
	end := time.UnixMilli(tg.Events[len(tg.Events)-1].Timestamp)
	return end.Sub(start)
}

func WithTraceGraph(ctx context.Context, tg *TraceGraph) context.Context {
	return context.WithValue(ctx, traceGraphKey{}, tg)
}

func GetTraceGraph(ctx context.Context) *TraceGraph {
	tg, _ := ctx.Value(traceGraphKey{}).(*TraceGraph)
	return tg
}
