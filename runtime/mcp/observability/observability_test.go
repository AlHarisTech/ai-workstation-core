package observability

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

func TestNewTraceEvent(t *testing.T) {
	evt := NewTraceEvent(EventGatewayRequest, "tr_1", "s1")
	if evt.Type != EventGatewayRequest {
		t.Fatalf("expected EventGatewayRequest, got %s", evt.Type)
	}
	if evt.TraceID != "tr_1" {
		t.Fatalf("expected trace_id tr_1, got %s", evt.TraceID)
	}
	if evt.Timestamp == 0 {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestTraceGraph_Basic(t *testing.T) {
	tg := NewTraceGraph()
	tg.Add(NewTraceEvent(EventGatewayRequest, "tr_1", "s1"))
	tg.Add(NewTraceEvent(EventAdapterComplete, "tr_1", "s1"))

	events := tg.Reconstruct()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
}

func TestTraceGraph_Duration(t *testing.T) {
	tg := NewTraceGraph()
	e1 := NewTraceEvent(EventGatewayRequest, "tr_1", "s1")
	e1.Timestamp = time.Now().UnixMilli()
	tg.Add(e1)
	time.Sleep(2 * time.Millisecond)
	e2 := NewTraceEvent(EventAdapterComplete, "tr_1", "s1")
	e2.Timestamp = time.Now().UnixMilli()
	tg.Add(e2)

	dur := tg.Duration()
	if dur <= 0 {
		t.Fatalf("expected positive duration, got %v", dur)
	}
	t.Logf("trace duration: %v", dur)
}

func TestTraceGraph_EmptyDuration(t *testing.T) {
	tg := NewTraceGraph()
	if tg.Duration() != 0 {
		t.Fatal("expected 0 duration for empty graph")
	}
}

func TestTraceGraph_SingleEventDuration(t *testing.T) {
	tg := NewTraceGraph()
	tg.Add(NewTraceEvent(EventGatewayRequest, "tr_1", "s1"))
	if tg.Duration() != 0 {
		t.Fatal("expected 0 duration for single event")
	}
}

func TestTraceGraph_ContextPropagation(t *testing.T) {
	tg := NewTraceGraph()
	ctx := WithTraceGraph(context.Background(), tg)

	got := GetTraceGraph(ctx)
	if got == nil {
		t.Fatal("expected trace graph from context")
	}

	got.Add(NewTraceEvent(EventGatewayRequest, "tr_1", "s1"))
	if len(tg.Events) != 1 {
		t.Fatalf("expected 1 event in original graph, got %d", len(tg.Events))
	}
}

func TestTraceGraph_NoGraphInContext(t *testing.T) {
	ctx := context.Background()
	got := GetTraceGraph(ctx)
	if got != nil {
		t.Fatal("expected nil for context without trace graph")
	}
}

func TestTraceGraph_ConcurrentSafe(t *testing.T) {
	tg := NewTraceGraph()
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tg.Add(NewTraceEvent(EventGatewayRequest, "tr_1", "s1"))
		}()
	}
	wg.Wait()
	events := tg.Reconstruct()
	if len(events) > maxTraceEvents {
		t.Fatalf("expected at most %d events (bounded), got %d", maxTraceEvents, len(events))
	}
}

func TestTelemetryCollector_SampleRate(t *testing.T) {
	tc := NewTelemetryCollector(1)
	counted := 0
	for i := 0; i < 100; i++ {
		if tc.ShouldSample() {
			counted++
		}
	}
	if counted != 100 {
		t.Fatalf("expected 100 samples at rate 1, got %d", counted)
	}
}

func TestTelemetryCollector_SampleRateDiv2(t *testing.T) {
	tc := NewTelemetryCollector(2)
	counted := 0
	for i := 0; i < 100; i++ {
		if tc.ShouldSample() {
			counted++
		}
	}
	if counted != 50 {
		t.Fatalf("expected 50 samples at rate 2, got %d", counted)
	}
}

func TestTelemetryCollector_ToolLatency(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tc.RecordToolLatency("filesystem", 10)
	tc.RecordToolLatency("filesystem", 20)
	tc.RecordToolLatency("filesystem", 30)

	snap := tc.Snapshot()
	avg, ok := snap.ToolLatency["filesystem"]
	if !ok {
		t.Fatal("expected filesystem latency in snapshot")
	}
	if avg != 20 {
		t.Fatalf("expected avg latency 20, got %d", avg)
	}
}

func TestTelemetryCollector_QueueDepth(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tc.RecordQueueDepth(5)
	snap := tc.Snapshot()
	if snap.QueueDepth != 5 {
		t.Fatalf("expected queue depth 5, got %d", snap.QueueDepth)
	}
}

func TestTelemetryCollector_CBState(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tc.RecordCBState("filesystem", "OPEN")
	tc.RecordCBState("filesystem", "OPEN")
	tc.RecordCBState("git", "CLOSED")

	snap := tc.Snapshot()
	if snap.CBStateDist["filesystem::OPEN"] != 2 {
		t.Fatalf("expected 2 filesystem::OPEN, got %d", snap.CBStateDist["filesystem::OPEN"])
	}
	if snap.CBStateDist["git::CLOSED"] != 1 {
		t.Fatalf("expected 1 git::CLOSED, got %d", snap.CBStateDist["git::CLOSED"])
	}
}

func TestTelemetryCollector_Counters(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tc.RecordRequest()
	tc.RecordRequest()
	tc.RecordReject()
	tc.RecordRetryConsumed()
	tc.RecordRetryConsumed()
	tc.RecordRetryConsumed()

	snap := tc.Snapshot()
	if snap.RequestTotal != 2 {
		t.Fatalf("expected 2 requests, got %d", snap.RequestTotal)
	}
	if snap.RejectTotal != 1 {
		t.Fatalf("expected 1 reject, got %d", snap.RejectTotal)
	}
	if snap.RetryConsumed != 3 {
		t.Fatalf("expected 3 retry consumed, got %d", snap.RetryConsumed)
	}
	if snap.RejectRate != 0.5 {
		t.Fatalf("expected 0.5 reject rate, got %f", snap.RejectRate)
	}
}

func TestTelemetryCollector_SaturationHits(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tc.RecordSaturationHit()
	tc.RecordSaturationHit()
	snap := tc.Snapshot()
	if snap.SaturationHits != 2 {
		t.Fatalf("expected 2 saturation hits, got %d", snap.SaturationHits)
	}
}

func TestControlSignals_NoSignals(t *testing.T) {
	snap := TelemetrySnapshot{RequestTotal: 100, RejectTotal: 0}
	signals := ComputeControlSignals(snap, 0.1, 0, 0)
	if len(signals) != 0 {
		t.Fatalf("expected 0 signals, got %d", len(signals))
	}
}

func TestControlSignals_SaturationWarning(t *testing.T) {
	snap := TelemetrySnapshot{}
	signals := ComputeControlSignals(snap, 0.9, 0, 0)
	if len(signals) < 1 {
		t.Fatal("expected at least saturation_warning signal")
	}
	hasSaturation := false
	for _, s := range signals {
		if s.Type == SignalSaturationWarning {
			hasSaturation = true
			if s.Severity != "warning" {
				t.Fatalf("expected warning severity for 0.9, got %s", s.Severity)
			}
		}
	}
	if !hasSaturation {
		t.Fatal("expected SignalSaturationWarning")
	}
}

func TestControlSignals_RoutingPressure(t *testing.T) {
	snap := TelemetrySnapshot{RequestTotal: 100, RejectTotal: 40}
	signals := ComputeControlSignals(snap, 0.1, 0, 0)
	hasPressure := false
	for _, s := range signals {
		if s.Type == SignalRoutingPressureHigh {
			hasPressure = true
		}
	}
	if !hasPressure {
		t.Fatal("expected SignalRoutingPressureHigh")
	}
}

func TestControlSignals_RetryDepletion(t *testing.T) {
	snap := TelemetrySnapshot{}
	signals := ComputeControlSignals(snap, 0.1, 0, 0.6)
	hasRetry := false
	for _, s := range signals {
		if s.Type == SignalRetryBudgetDepletion {
			hasRetry = true
		}
	}
	if !hasRetry {
		t.Fatal("expected SignalRetryBudgetDepletion")
	}
}

func TestControlSignals_ToolDegradation(t *testing.T) {
	snap := TelemetrySnapshot{}
	signals := ComputeControlSignals(snap, 0.1, 3, 0)
	hasDegradation := false
	for _, s := range signals {
		if s.Type == SignalToolHealthDegradation {
			hasDegradation = true
			if s.Value != 3 {
				t.Fatalf("expected value 3 for degraded tools, got %f", s.Value)
			}
		}
	}
	if !hasDegradation {
		t.Fatal("expected SignalToolHealthDegradation")
	}
}

func TestHealthScores_Perfect(t *testing.T) {
	snap := TelemetrySnapshot{RequestTotal: 100, RejectTotal: 0}
	scores := ComputeHealthScores(snap, 0.0, 0, 3)
	if scores.ToolHealthScore != 1.0 {
		t.Fatalf("expected tool health 1.0, got %f", scores.ToolHealthScore)
	}
	if scores.RouterStabilityScore != 1.0 {
		t.Fatalf("expected router stability 1.0, got %f", scores.RouterStabilityScore)
	}
	if scores.GatewayLoadScore != 1.0 {
		t.Fatalf("expected gateway load 1.0, got %f", scores.GatewayLoadScore)
	}
	if scores.SystemSaturationIndex != 0.0 {
		t.Fatalf("expected saturation 0.0, got %f", scores.SystemSaturationIndex)
	}
	if scores.OverallHealth != 1.0 {
		t.Fatalf("expected overall health 1.0, got %f", scores.OverallHealth)
	}
}

func TestHealthScores_Degraded(t *testing.T) {
	snap := TelemetrySnapshot{RequestTotal: 100, RejectTotal: 50}
	scores := ComputeHealthScores(snap, 0.9, 2, 3)
	expectedToolHealth := 1.0 - float64(2)/float64(3)
	if abs(scores.ToolHealthScore-expectedToolHealth) > 1e-9 {
		t.Fatalf("expected tool health %f, got %f, diff=%e", expectedToolHealth, scores.ToolHealthScore, abs(scores.ToolHealthScore-expectedToolHealth))
	}
	if scores.RouterStabilityScore != 0.2 {
		t.Fatalf("expected router stability 0.2, got %f", scores.RouterStabilityScore)
	}
	if scores.OverallHealth < 0 || scores.OverallHealth > 1 {
		t.Fatalf("overall health out of range: %f", scores.OverallHealth)
	}
	t.Logf("degraded health scores: %+v", scores)
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func TestHealthScores_Empty(t *testing.T) {
	snap := TelemetrySnapshot{}
	scores := ComputeHealthScores(snap, 0, 0, 0)
	if scores.ToolHealthScore != 1.0 {
		t.Fatalf("expected tool health 1.0 for zero tools, got %f", scores.ToolHealthScore)
	}
}

func TestInstrumentedAdapter_EmitsEvents(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tg := NewTraceGraph()
	ctx := WithTraceGraph(context.Background(), tg)

	inner := &mockAdapter{name: "mock", success: true}
	ia := NewInstrumentedAdapter(inner, tc)

	resp, err := ia.Execute(ctx, mockRequest())
	if err != nil {
		t.Fatalf("expected no error: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}

	events := tg.Reconstruct()
	if len(events) != 2 {
		t.Fatalf("expected 2 events (execute + complete), got %d", len(events))
	}
	if events[0].Type != EventAdapterExecute {
		t.Fatalf("expected EventAdapterExecute, got %s", events[0].Type)
	}
	if events[1].Type != EventAdapterComplete {
		t.Fatalf("expected EventAdapterComplete, got %s", events[1].Type)
	}
}

func TestInstrumentedAdapter_RecordsFailure(t *testing.T) {
	tc := NewTelemetryCollector(1)
	tg := NewTraceGraph()
	ctx := WithTraceGraph(context.Background(), tg)

	inner := &mockAdapter{name: "mock", success: false}
	ia := NewInstrumentedAdapter(inner, tc)

	resp, _ := ia.Execute(ctx, mockRequest())
	if resp.Success {
		t.Fatal("expected failure")
	}

	events := tg.Reconstruct()
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[1].Status != "fail" {
		t.Fatalf("expected fail status, got %s", events[1].Status)
	}
}

func TestInstrumentedAdapter_LatencyRecorded(t *testing.T) {
	tc := NewTelemetryCollector(1)

	inner := &mockAdapter{name: "fs", success: true, delay: 2 * time.Millisecond}
	ia := NewInstrumentedAdapter(inner, tc)

	ia.Execute(context.Background(), mockRequest())

	snap := tc.Snapshot()
	avg, ok := snap.ToolLatency["filesystem"]
	if !ok {
		t.Fatal("expected tool latency recorded")
	}
	if avg < 1 {
		t.Fatalf("expected latency >= 1ms, got %d", avg)
	}
}

func TestTelemetrySnapshot_Empty(t *testing.T) {
	tc := NewTelemetryCollector(1)
	snap := tc.Snapshot()
	if snap.RequestTotal != 0 {
		t.Fatal("expected 0 requests")
	}
	if snap.RejectRate != 0.0 {
		t.Fatal("expected 0 reject rate")
	}
}

// ---- Mock helpers ----

type mockAdapter struct {
	name    string
	success bool
	delay   time.Duration
}

func (m *mockAdapter) Name() string { return m.name }

func (m *mockAdapter) Execute(_ context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	if !m.success {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "mock fail"}, nil
	}
	return types.MCPResponse{ID: req.ID, Success: true}, nil
}

func mockRequest() types.MCPRequest {
	return types.MCPRequest{ID: "test_1", SessionID: "s1", Tool: "filesystem", Action: "read"}
}

// ---- Efficiency Tests ----

func TestTraceGraph_BoundedRetention(t *testing.T) {
	tg := NewTraceGraph()
	for i := 0; i < 20; i++ {
		tg.Add(NewTraceEvent(EventGatewayRequest, "tr_1", "s1"))
	}
	events := tg.Reconstruct()
	if len(events) != maxTraceEvents {
		t.Fatalf("expected %d events (bounded), got %d", maxTraceEvents, len(events))
	}
}

func TestLatencyBudget_UnderLimit(t *testing.T) {
	lb := NewLatencyBudget(1 * time.Second)
	start := time.Now()
	if err := lb.Check(start); err != nil {
		t.Fatalf("expected no error under limit: %v", err)
	}
}

func TestLatencyBudget_Exceeded(t *testing.T) {
	lb := NewLatencyBudget(1 * time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if err := lb.Check(time.Now().Add(-10 * time.Millisecond)); err == nil {
		t.Fatal("expected error when budget exceeded")
	} else {
		t.Logf("budget exceeded: %v", err)
	}
}

func TestFastPath_ShouldSkipTrace(t *testing.T) {
	fc := DefaultFastPathConfig()
	if !fc.ShouldSkipTrace(0.1) {
		t.Fatal("expected skip trace at 10% load")
	}
	if fc.ShouldSkipTrace(0.5) {
		t.Fatal("expected no skip trace at 50% load")
	}
}

func TestFastPath_Disabled(t *testing.T) {
	fc := FastPathConfig{traceEnabled: false}
	if !fc.ShouldSkipTrace(0.9) {
		t.Fatal("expected skip trace when disabled")
	}
}

func TestSessionTracker_Touch(t *testing.T) {
	st := NewSessionTracker(10, 1*time.Hour)
	st.Touch("s1")
	if st.ActiveCount() != 1 {
		t.Fatalf("expected 1 active session, got %d", st.ActiveCount())
	}
}

func TestSessionTracker_IsStale(t *testing.T) {
	st := NewSessionTracker(10, 50*time.Millisecond)
	st.Touch("s1")
	if st.IsStale("s1") {
		t.Fatal("expected session not stale immediately")
	}
	time.Sleep(60 * time.Millisecond)
	if !st.IsStale("s1") {
		t.Fatal("expected session stale after timeout")
	}
}

func TestSessionTracker_EvictStale(t *testing.T) {
	st := NewSessionTracker(10, 50*time.Millisecond)
	st.Touch("s1")
	st.Touch("s2")
	time.Sleep(60 * time.Millisecond)
	evicted := st.EvictStale()
	if evicted != 2 {
		t.Fatalf("expected 2 evicted, got %d", evicted)
	}
}

func TestSessionTracker_MaxSessions(t *testing.T) {
	st := NewSessionTracker(3, 1*time.Hour)
	for i := 0; i < 3; i++ {
		st.Touch(fmt.Sprintf("s%d", i))
	}
	if st.CanAccept() {
		t.Fatal("expected cannot accept at max sessions")
	}
}

func TestSessionTracker_CanAccept(t *testing.T) {
	st := NewSessionTracker(10, 1*time.Hour)
	if !st.CanAccept() {
		t.Fatal("expected can accept under limit")
	}
}

func TestKernelSignalReader_Basic(t *testing.T) {
	r := NewKernelSignalReader(
		func() float64 { return 0.5 },
		func() float64 { return 0.9 },
	)
	if r.SaturationPct() != 0.5 {
		t.Fatalf("expected 0.5 saturation, got %f", r.SaturationPct())
	}
	if r.HealthScore() != 0.9 {
		t.Fatalf("expected 0.9 health, got %f", r.HealthScore())
	}
}

func TestKernelSignalReader_NilFns(t *testing.T) {
	r := NewKernelSignalReader(nil, nil)
	if r.SaturationPct() != 0 {
		t.Fatal("expected 0 saturation for nil fn")
	}
	if r.HealthScore() != 1.0 {
		t.Fatal("expected 1.0 health for nil fn")
	}
}

func TestToolLatencyStats_RingBuffer(t *testing.T) {
	s := newToolLatencyStats(3)
	s.record(10)
	s.record(20)
	s.record(30)
	s.record(40)

	avg, count := s.snapshot()
	if count != 3 {
		t.Fatalf("expected 3 samples, got %d", count)
	}
	if avg != 30 {
		t.Fatalf("expected avg 30 (40+20+30)/3, got %d", avg)
	}
}

var _ = fmt.Sprintf
var _ = sync.WaitGroup{}
