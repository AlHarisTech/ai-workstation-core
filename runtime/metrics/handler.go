package metrics

import (
	"encoding/json"
	"net/http"
	"strings"
)

type DashboardHandler struct {
	registry *MetricsRegistry
	aggregator *TraceAggregator
}

func NewDashboardHandler(reg *MetricsRegistry, agg *TraceAggregator) *DashboardHandler {
	if reg == nil {
		reg = Global()
	}
	if agg == nil {
		agg = NewTraceAggregator()
	}
	return &DashboardHandler{
		registry:   reg,
		aggregator: agg,
	}
}

func (dh *DashboardHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	path := strings.TrimPrefix(r.URL.Path, "/")
	switch path {
	case "metrics", "metrics/":
		dh.handleRoot(w, r)
	case "metrics/runtime":
		dh.handleRuntime(w, r)
	case "metrics/gateway":
		dh.handleGateway(w, r)
	case "metrics/stages":
		dh.handleStages(w, r)
	case "metrics/enforcement":
		dh.handleEnforcement(w, r)
	case "metrics/learning":
		dh.handleLearning(w, r)
	case "metrics/policy-intel":
		dh.handlePolicyIntel(w, r)
	case "metrics/traces":
		dh.handleTraces(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (dh *DashboardHandler) handleRoot(w http.ResponseWriter, _ *http.Request) {
	snap := dh.registry.Snapshot()
	json.NewEncoder(w).Encode(snap)
}

func (dh *DashboardHandler) handleRuntime(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(dh.registry.Snapshot().Runtime)
}

func (dh *DashboardHandler) handleGateway(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(dh.registry.Snapshot().Gateway)
}

func (dh *DashboardHandler) handleStages(w http.ResponseWriter, _ *http.Request) {
	stages := dh.registry.Snapshot().Stages
	lat := dh.aggregator.StageLatencyStats()
	json.NewEncoder(w).Encode(map[string]any{
		"stage_counters": stages,
		"stage_latency":  lat,
	})
}

func (dh *DashboardHandler) handleEnforcement(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(dh.registry.Snapshot().Enforcement)
}

func (dh *DashboardHandler) handleLearning(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(dh.registry.Snapshot().Learning)
}

func (dh *DashboardHandler) handlePolicyIntel(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(dh.registry.Snapshot().PolicyIntel)
}

func (dh *DashboardHandler) handleTraces(w http.ResponseWriter, _ *http.Request) {
	json.NewEncoder(w).Encode(map[string]any{
		"count":       dh.aggregator.TraceCount(),
		"bytes_used":  dh.aggregator.ByteUsage(),
		"traces":      dh.aggregator.Traces(),
	})
}
