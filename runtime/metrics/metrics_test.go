package metrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRegistryCounters(t *testing.T) {
	reg := NewMetricsRegistry()

	reg.RecordRequest(true)
	reg.RecordRequest(true)
	reg.RecordRequest(false)
	reg.RecordFailure()
	reg.RecordRateLimit()

	snap := reg.Snapshot()
	if snap.Gateway.RequestsTotal != 3 {
		t.Errorf("RequestsTotal = %d, want 3", snap.Gateway.RequestsTotal)
	}
	if snap.Gateway.RequestsAllowed != 2 {
		t.Errorf("RequestsAllowed = %d, want 2", snap.Gateway.RequestsAllowed)
	}
	if snap.Gateway.RequestsBlocked != 1 {
		t.Errorf("RequestsBlocked = %d, want 1", snap.Gateway.RequestsBlocked)
	}
	if snap.Gateway.RequestsFailed != 1 {
		t.Errorf("RequestsFailed = %d, want 1", snap.Gateway.RequestsFailed)
	}
	if snap.Gateway.RateLimited != 1 {
		t.Errorf("RateLimited = %d, want 1", snap.Gateway.RateLimited)
	}
}

func TestMetricsRegistryEnforcement(t *testing.T) {
	reg := NewMetricsRegistry()

	reg.RecordEnforcement(true)
	reg.RecordEnforcement(true)
	reg.RecordEnforcement(false)
	reg.RecordViolation()

	snap := reg.Snapshot()
	if snap.Enforcement.Evaluations != 3 {
		t.Errorf("Evaluations = %d, want 3", snap.Enforcement.Evaluations)
	}
	if snap.Enforcement.Allowed != 2 {
		t.Errorf("Allowed = %d, want 2", snap.Enforcement.Allowed)
	}
	if snap.Enforcement.Blocked != 1 {
		t.Errorf("Blocked = %d, want 1", snap.Enforcement.Blocked)
	}
	if snap.Enforcement.Violations != 1 {
		t.Errorf("Violations = %d, want 1", snap.Enforcement.Violations)
	}
}

func TestMetricsRegistryPolicyIntel(t *testing.T) {
	reg := NewMetricsRegistry()

	reg.RecordPolicyEvent()
	reg.RecordDriftDetection()
	reg.RecordSuggestion()
	reg.RecordWeightUpdate()

	snap := reg.Snapshot()
	if snap.PolicyIntel.EventsRecorded != 1 {
		t.Errorf("EventsRecorded = %d, want 1", snap.PolicyIntel.EventsRecorded)
	}
	if snap.PolicyIntel.DriftDetections != 1 {
		t.Errorf("DriftDetections = %d, want 1", snap.PolicyIntel.DriftDetections)
	}
	if snap.PolicyIntel.Suggestions != 1 {
		t.Errorf("Suggestions = %d, want 1", snap.PolicyIntel.Suggestions)
	}
	if snap.PolicyIntel.WeightUpdates != 1 {
		t.Errorf("WeightUpdates = %d, want 1", snap.PolicyIntel.WeightUpdates)
	}
}

func TestMetricsRegistryLearning(t *testing.T) {
	reg := NewMetricsRegistry()

	reg.RecordLearningUpdate(true, 10)
	reg.RecordLearningUpdate(true, 20)
	reg.RecordLearningUpdate(false, 30)

	snap := reg.Snapshot()
	if snap.Learning.Updates != 3 {
		t.Errorf("Updates = %d, want 3", snap.Learning.Updates)
	}
	if snap.Learning.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", snap.Learning.SuccessCount)
	}
	if snap.Learning.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", snap.Learning.FailureCount)
	}
	// avg = (10+20+30)/3 = 20
	if snap.Learning.AvgLatencyMs != 20 {
		t.Errorf("AvgLatencyMs = %d, want 20", snap.Learning.AvgLatencyMs)
	}
}

func TestRuntimeMetricsStage(t *testing.T) {
	rm := NewRuntimeMetrics()

	s1 := rm.Stage("validate")
	s1.Invocations.Add(5)
	s1.TotalLatency.Add(250)

	s2 := rm.Stage("resolve")
	s2.Invocations.Add(3)

	if len(rm.Stages) != 2 {
		t.Errorf("Stages count = %d, want 2", len(rm.Stages))
	}
	if rm.Stage("validate") != s1 {
		t.Error("Stage() did not return same instance")
	}
}

func TestSnapshotJSON(t *testing.T) {
	reg := NewMetricsRegistry()
	reg.RecordRequest(true)
	reg.RecordRequest(true)
	reg.RecordRequest(false)

	data, err := reg.SnapshotJSON()
	if err != nil {
		t.Fatalf("SnapshotJSON() error: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("JSON unmarshal error: %v", err)
	}

	gateway, ok := parsed["gateway"].(map[string]any)
	if !ok {
		t.Fatal("gateway field missing in JSON")
	}
	if gateway["requests_total"].(float64) != 3 {
		t.Errorf("requests_total = %v, want 3", gateway["requests_total"])
	}
	if gateway["requests_allowed"].(float64) != 2 {
		t.Errorf("requests_allowed = %v, want 2", gateway["requests_allowed"])
	}
	if gateway["requests_blocked"].(float64) != 1 {
		t.Errorf("requests_blocked = %v, want 1", gateway["requests_blocked"])
	}
}

func TestDashboardHandlerRoutes(t *testing.T) {
	reg := NewMetricsRegistry()
	agg := NewTraceAggregator()
	handler := NewDashboardHandler(reg, agg)

	tests := []struct {
		path   string
		status int
	}{
		{"/metrics", 200},
		{"/metrics/runtime", 200},
		{"/metrics/gateway", 200},
		{"/metrics/stages", 200},
		{"/metrics/enforcement", 200},
		{"/metrics/learning", 200},
		{"/metrics/policy-intel", 200},
		{"/metrics/traces", 200},
		{"/metrics/nonexistent", 404},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != tt.status {
			t.Errorf("%s: got status %d, want %d", tt.path, w.Code, tt.status)
		}
	}
}

func TestDashboardHandlerContentType(t *testing.T) {
	handler := NewDashboardHandler(nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestTraceAggregatorRecord(t *testing.T) {
	agg := NewTraceAggregator()

	agg.RecordTrace("trace-1", "req-1", "git", []string{"validate", "policy", "route"}, 1000)
	agg.RecordTrace("trace-2", "req-2", "fetch", []string{"validate", "policy", "route"}, 2000)

	traces := agg.Traces()
	if len(traces) != 2 {
		t.Errorf("Trace count = %d, want 2", len(traces))
	}
	if traces[0].TraceID != "trace-1" || traces[0].SelectedServer != "git" {
		t.Error("First trace data mismatch")
	}
	if traces[1].TraceID != "trace-2" || traces[1].SelectedServer != "fetch" {
		t.Error("Second trace data mismatch")
	}
}

func TestTraceAggregatorMaxSize(t *testing.T) {
	agg := NewTraceAggregator()

	longID := ""
	for i := 0; i < MaxTraceFieldSize+100; i++ {
		longID += "x"
	}

	agg.RecordTrace(longID, "req-1", "git", []string{"s1"}, 1000)
	traces := agg.Traces()
	if len(traces) != 1 {
		t.Errorf("Trace count = %d, want 1", len(traces))
	}
	if len(traces[0].TraceID) > MaxTraceFieldSize {
		t.Errorf("TraceID truncated to %d, want <= %d", len(traces[0].TraceID), MaxTraceFieldSize)
	}
}

func TestTraceAggregatorTotalLimit(t *testing.T) {
	agg := NewTraceAggregator()

	for i := 0; i < 2000; i++ {
		agg.RecordTrace(
			fmt.Sprintf("trace-%d", i),
			fmt.Sprintf("req-%d", i),
			"git",
			[]string{"validate", "policy", "resolve", "knowledge", "route", "enforcement", "execute"},
			1000,
		)
	}

	if count := agg.TraceCount(); count > MaxAggregatedTraces {
		t.Errorf("Trace count = %d, exceeded max %d", count, MaxAggregatedTraces)
	}
	if agg.ByteUsage() <= 0 {
		t.Error("ByteUsage() should be positive")
	}
}

func TestTraceAggregatorStageLatency(t *testing.T) {
	agg := NewTraceAggregator()

	agg.RecordStageLatency("validate", 25)
	agg.RecordStageLatency("validate", 35)
	agg.RecordStageLatency("enforcement", 50)

	stats := agg.StageLatencyStats()
	if len(stats) != 2 {
		t.Errorf("Stage latency stats count = %d, want 2", len(stats))
	}

	for _, s := range stats {
		switch s.Stage {
		case "validate":
			if s.Count != 2 {
				t.Errorf("validate count = %d, want 2", s.Count)
			}
			if s.TotalNs != 60 {
				t.Errorf("validate TotalNs = %d, want 60", s.TotalNs)
			}
			if s.MinNs != 25 {
				t.Errorf("validate MinNs = %d, want 25", s.MinNs)
			}
			if s.MaxNs != 35 {
				t.Errorf("validate MaxNs = %d, want 35", s.MaxNs)
			}
		case "enforcement":
			if s.Count != 1 {
				t.Errorf("enforcement count = %d, want 1", s.Count)
			}
		}
	}
}

func TestCLIDashboard(t *testing.T) {
	reg := NewMetricsRegistry()
	reg.RecordRequest(true)
	reg.RecordEnforcement(true)
	reg.RecordPolicyEvent()

	dash := NewCLIDashboard(reg)
	output := dash.Render()
	if len(output) == 0 {
		t.Error("CLI dashboard output is empty")
	}
	if !strings.Contains(output, "Gateway Metrics Dashboard") {
		t.Error("Dashboard missing title header")
	}
	if !strings.Contains(output, "Gateway Counters") {
		t.Error("Dashboard missing gateway counters")
	}
	if !strings.Contains(output, "Total:") {
		t.Error("Dashboard missing total counter")
	}

	compact := dash.RenderCompact()
	if len(compact) == 0 {
		t.Error("Compact dashboard output is empty")
	}
}

func TestGlobalRegistry(t *testing.T) {
	ResetGlobal()
	r := Global()
	if r == nil {
		t.Fatal("Global() returned nil")
	}
	if r.Runtime == nil {
		t.Fatal("Global().Runtime is nil")
	}

	r2 := Global()
	if r2 != r {
		t.Error("Global() should return same instance")
	}

	ResetGlobal()
	r3 := Global()
	if r3 == r {
		t.Error("After ResetGlobal(), Global() should return new instance")
	}
}

func TestRuntimeMetricsDerived(t *testing.T) {
	rm := NewRuntimeMetrics()
	if rm.ThroughputRPS() != 0 {
		t.Error("ThroughputRPS should be 0 for idle runtime")
	}
	if rm.BlockRate() != 0 {
		t.Error("BlockRate should be 0 for idle runtime")
	}
	if rm.ErrorRate() != 0 {
		t.Error("ErrorRate should be 0 for idle runtime")
	}

	rm.Gateway.RequestsTotal.Add(100)
	rm.Gateway.RequestsBlocked.Add(10)
	rm.Gateway.RequestsFailed.Add(5)

	if rm.BlockRate() != 0.1 {
		t.Errorf("BlockRate = %f, want 0.1", rm.BlockRate())
	}
	if rm.ErrorRate() != 0.05 {
		t.Errorf("ErrorRate = %f, want 0.05", rm.ErrorRate())
	}
}


