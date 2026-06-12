package mcpv2

import (
	"testing"
	"time"
)

// Global baseline gateway for benchmarks — set up once, reused across all
// sub-benchmarks to avoid repeated construction overhead.
var baselineGateway *Gateway

func init() {
	baselineGateway = NewGateway()
}

// ---------------------------------------------------------------------------
// Stage-level benchmarks — measure each pipeline stage in isolation
// ---------------------------------------------------------------------------

func BenchmarkStageValidate(b *testing.B) {
	req := validRequest("/tmp")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ValidateRequest(req)
	}
}

func BenchmarkStagePolicy(b *testing.B) {
	req := validRequest("/tmp")
	req.Policy.Allow = []string{"git.*"}
	gw := NewGateway()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gw.policy.Enforce(req.Action.Type, req.Action.Operation, req.Policy)
	}
}

func BenchmarkStageResolve(b *testing.B) {
	gw := NewGateway()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gw.router.Resolve(ActionGit, "status")
	}
}

func BenchmarkStageScoreSelect(b *testing.B) {
	gw := NewGateway()
	candidates := gw.router.ListAll()
	req := validRequest("/tmp")
	trace := &DecisionTrace{TraceID: "bench", RequestID: "bench"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gw.selectBestServer(candidates, req, nil, trace)
	}
}

func BenchmarkStageScoreSelectWithKnowledge(b *testing.B) {
	gw := NewGateway()
	candidates := gw.router.ListAll()
	req := validRequest("/tmp")
	trace := &DecisionTrace{TraceID: "bench", RequestID: "bench"}
	knowledge := []KnowledgeDoc{
		{Collection: "mcp", Query: "git.status", Results: map[string]any{"documents": []any{"git is the version control tool"}}},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gw.selectBestServer(candidates, req, knowledge, trace)
	}
}

func BenchmarkStageEnforcementDefault(b *testing.B) {
	ee := NewEnforcementEngine()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ee.Check("git", "git.status")
	}
}

func BenchmarkStageEnforcementWithRule(b *testing.B) {
	ee := NewEnforcementEngine()
	ee.SetRule("git", "git.status", false, "deny for benchmark")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ee.Check("git", "git.status")
	}
}

func BenchmarkStageLearningUpdate(b *testing.B) {
	le := NewLearningEngine()
	outcome := RoutingOutcome{
		RequestID:      "bench",
		SelectedServer: "git",
		Success:        true,
		LatencyMs:      10,
		Timestamp:      time.Now(),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		le.Update(outcome)
	}
}

func BenchmarkStageStabilityAdjust(b *testing.B) {
	se := NewStabilityEngine(0.02, 20)
	// Prime the engine with some history
	for i := 0; i < 10; i++ {
		se.RecordSelection("status", "git")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = se.AdjustScore("git", "status", 0.75)
	}
}

func BenchmarkStageTracePopulation(b *testing.B) {
	trace := &DecisionTrace{TraceID: "bench", RequestID: "bench"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		trace.Steps = append(trace.Steps, TraceStep{
			Stage:  "validate",
			Output: "ok",
		})
	}
}

// ---------------------------------------------------------------------------
// Full gateway process benchmarks — end-to-end with various configurations
// ---------------------------------------------------------------------------

func BenchmarkGatewayProcess(b *testing.B) {
	workspace := "/tmp"
	req := validRequest(workspace)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = baselineGateway.Process(req)
	}
}

func BenchmarkGatewayProcessWithEnforcement(b *testing.B) {
	gw := NewGateway()
	gw.enforcement.SetRule("git", "git.status", true, "allow")
	req := validRequest("/tmp")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gw.Process(req)
	}
}

func BenchmarkGatewayProcessBlocked(b *testing.B) {
	gw := NewGateway()
	gw.enforcement.SetRule("git", "git.status", false, "blocked for benchmark")
	req := validRequest("/tmp")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gw.Process(req)
	}
}

func BenchmarkGatewayProcessWithKnowledge(b *testing.B) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Context.Knowledge = []KnowledgeDoc{
		{
			Collection: "mcp",
			Query:      "git.status",
			Results:    map[string]any{"documents": []any{"git is the version control system for code management"}},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gw.Process(req)
	}
}

func BenchmarkGatewayProcessWithStability(b *testing.B) {
	gw := NewGateway()
	// Pre-warm the stability engine with selections to test adjustment overhead
	for i := 0; i < 50; i++ {
		gw.stability.RecordSelection("status", "git")
	}
	req := validRequest("/tmp")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = gw.Process(req)
	}
}

// ---------------------------------------------------------------------------
// Policy Intelligence benchmarks
// ---------------------------------------------------------------------------

func BenchmarkPolicyIntelligenceRecord(b *testing.B) {
	pie := NewPolicyIntelligenceEngine()
	event := PolicyEvent{
		TraceID:   "bench",
		RequestID: "bench",
		Server:    "git",
		Operation: "git.status",
		Allowed:   true,
		Blocked:   false,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pie.Record(event)
	}
}

func BenchmarkPolicyIntelligenceDriftDetection(b *testing.B) {
	pie := NewPolicyIntelligenceEngine()
	// Record 10 events to create drift state
	for i := 0; i < 10; i++ {
		pie.Record(PolicyEvent{
			TraceID:   "bench",
			RequestID: "bench",
			Server:    "git",
			Operation: "git.status",
			Allowed:   false,
			Blocked:   true,
			Reason:    "benchmark",
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = pie.DetectDrift("git", "git.status")
	}
}

func BenchmarkPolicyIntelligenceSuggestions(b *testing.B) {
	pie := NewPolicyIntelligenceEngine()
	for i := 0; i < 20; i++ {
		pie.Record(PolicyEvent{
			TraceID:   "bench",
			RequestID: "bench",
			Server:    "git",
			Operation: "git.status",
			Allowed:   false,
			Blocked:   true,
			Reason:    "benchmark",
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = pie.GenerateSuggestions()
	}
}

// ---------------------------------------------------------------------------
// Stability engine convergence benchmarks
// ---------------------------------------------------------------------------

func BenchmarkStabilityEngineConvergence(b *testing.B) {
	se := NewStabilityEngine(0.02, 20)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		se.RecordSelection("status", "git")
		se.RecordSelection("status", "git")
		se.RecordSelection("status", "git")
		se.RecordSelection("status", "git")
		se.RecordSelection("status", "git")
	}
}

func BenchmarkStabilityEngineOscillation(b *testing.B) {
	se := NewStabilityEngine(0.02, 20)
	servers := []string{"git", "fetch", "git", "fetch", "git"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, s := range servers {
			se.RecordSelection("status", s)
		}
	}
}

// ---------------------------------------------------------------------------
// Exploration state benchmarks
// ---------------------------------------------------------------------------

func BenchmarkExplorationAdjustScore(b *testing.B) {
	es := NewExplorationState(0.10)
	for i := 0; i < 50; i++ {
		es.RecordSelection("git")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = es.AdjustScore("git", 0.75)
	}
}

func BenchmarkExplorationEffectiveRate(b *testing.B) {
	es := NewExplorationState(0.10)
	for i := 0; i < 50; i++ {
		es.RecordSelection("git")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = es.AdjustScoreWithRate("git", 0.75, 0.10)
	}
}
