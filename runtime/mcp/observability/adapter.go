package observability

import (
	"context"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type InstrumentedAdapter struct {
	inner      types.MCPAdapter
	telemetry  *TelemetryCollector
	fastPathFn func() float64
}

func NewInstrumentedAdapter(inner types.MCPAdapter, tc *TelemetryCollector) *InstrumentedAdapter {
	return &InstrumentedAdapter{
		inner:     inner,
		telemetry: tc,
	}
}

func (ia *InstrumentedAdapter) Name() string {
	return "instrumented:" + ia.inner.Name()
}

func (ia *InstrumentedAdapter) SetFastPathFn(fn func() float64) {
	ia.fastPathFn = fn
}

func (ia *InstrumentedAdapter) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	tg := GetTraceGraph(ctx)
	start := time.Now()

	fastPath := false
	if ia.fastPathFn != nil && ia.fastPathFn() < 0.3 {
		fastPath = true
	}

	if !fastPath && tg != nil {
		tg.Add(NewTraceEvent(EventAdapterExecute, req.ID, req.SessionID))
	}

	resp, err := ia.inner.Execute(ctx, req)

	latencyMS := time.Since(start).Milliseconds()

	if !fastPath && tg != nil {
		evt := NewTraceEvent(EventAdapterComplete, req.ID, req.SessionID)
		evt.Tool = req.Tool
		evt.Action = req.Action
		evt.LatencyMS = latencyMS
		evt.Status = "ok"
		if err != nil || !resp.Success {
			evt.Status = "fail"
			evt.Error = resp.Error
		}
		tg.Add(evt)
	}

	if ia.telemetry != nil && ia.telemetry.ShouldSample() {
		ia.telemetry.RecordToolLatency(req.Tool, latencyMS)
	}

	return resp, err
}
