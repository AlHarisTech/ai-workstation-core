package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/contracts"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/hardening"
	mcpobs "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/observability"
	mcprouter "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/router"
	mcptypes "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/tools/filesystem"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/tools/git"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/tools/github"
)

type IntegrationGateway struct {
	kernel           *kernel.KernelEngine
	router           *mcprouter.ToolRouter
	backpressure     *contracts.BackpressureModel
	telemetry        *mcpobs.TelemetryCollector
	latencyBudget    *mcpobs.LatencyBudget
	sessionTracker   *mcpobs.SessionTracker
	kernelSignals    *mcpobs.KernelSignalReader
}

func NewIntegrationGateway(ke *kernel.KernelEngine) *IntegrationGateway {
	ig := &IntegrationGateway{
		kernel:         ke,
		router:         mcprouter.NewToolRouter(),
		backpressure:   contracts.NewBackpressureModel(contracts.DefaultBackpressureConfig()),
		telemetry:      mcpobs.NewTelemetryCollector(1),
		latencyBudget:  mcpobs.NewLatencyBudget(30 * time.Second),
		sessionTracker: mcpobs.NewSessionTracker(100, 30*time.Minute),
	}

	ig.kernelSignals = mcpobs.NewKernelSignalReader(
		func() float64 {
			return float64(ig.backpressure.ActiveCount()) / float64(ig.backpressure.Config().MaxQueueTotal)
		},
		func() float64 {
			return ig.HealthSnapshot().OverallHealth
		},
	)

	ig.registerWrapped("filesystem", "read", filesystem.New())
	ig.registerWrapped("filesystem", "write", filesystem.New())
	ig.registerWrapped("filesystem", "list", filesystem.New())
	ig.registerWrapped("filesystem", "search", filesystem.New())
	ig.registerWrapped("git", "status", git.New("."))
	ig.registerWrapped("git", "diff", git.New("."))
	ig.registerWrapped("git", "log", git.New("."))
	ig.registerWrapped("git", "branch", git.New("."))
	ig.registerWrapped("github", "create_pr", github.New())
	ig.registerWrapped("github", "list_issues", github.New())
	ig.registerWrapped("github", "create_issue", github.New())

	return ig
}

func (ig *IntegrationGateway) registerWrapped(tool, action string, adapter mcptypes.MCPAdapter) {
	adapter = contracts.NewCircuitBreaker(adapter, contracts.DefaultCircuitBreakerConfig())
	adapter = contracts.NewTimeoutAdapter(adapter, 30*time.Second)
	adapter = contracts.NewRetryAdapter(adapter, contracts.DefaultRetryConfig())
	instrumented := mcpobs.NewInstrumentedAdapter(adapter, ig.telemetry)
	instrumented.SetFastPathFn(func() float64 { return ig.kernelSignals.SaturationPct() })
	ig.router.Register(tool, action, instrumented)
}

func (ig *IntegrationGateway) BackpressureModel() *contracts.BackpressureModel {
	return ig.backpressure
}

func (ig *IntegrationGateway) SetBackpressureConfig(cfg contracts.BackpressureConfig) {
	ig.backpressure = contracts.NewBackpressureModel(cfg)
}

func (ig *IntegrationGateway) Process(raw json.RawMessage) mcptypes.MCPResponse {
	start := time.Now()

	var req mcptypes.MCPRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return mcptypes.NewResponse("parse_error", "", false, nil, err.Error(), 0)
	}

	if req.Timestamp == 0 {
		req.Timestamp = time.Now().UnixMilli()
	}

	traceID := req.ID
	tg := mcpobs.NewTraceGraph()
	ctx := mcpobs.WithTraceGraph(context.Background(), tg)

	ig.telemetry.RecordRequest()
	ig.sessionTracker.Touch(req.SessionID)

	tg.Add(mcpobs.NewTraceEvent(mcpobs.EventGatewayRequest, traceID, req.SessionID))

	// Step 1: Synchronous session validation
	if req.SessionID == "" || req.ProjectID == "" {
		ig.telemetry.RecordReject()
		return mcptypes.NewResponse(req.ID, "", false, nil, "SESSION_INVALID: session_id and project_id required", time.Since(start).Milliseconds())
	}

	// Step 2: Latency budget check
	if err := ig.latencyBudget.Check(start); err != nil {
		ig.telemetry.RecordReject()
		return mcptypes.NewResponse(req.ID, "", false, nil, err.Error(), time.Since(start).Milliseconds())
	}

	// Step 3: Backpressure check
	if ig.backpressure.IsSaturated() {
		ig.telemetry.RecordSaturationHit()
		ig.telemetry.RecordReject()
		tg.Add(mcpobs.NewTraceEvent(mcpobs.EventBackpressureHit, traceID, req.SessionID))
		return mcptypes.NewResponse(req.ID, "", false, nil, "BACKPRESSURE_SATURATED: system at capacity", time.Since(start).Milliseconds())
	}
	if ig.backpressure.IsSoftRejected(req.SessionID) {
		ig.telemetry.RecordReject()
		tg.Add(mcpobs.NewTraceEvent(mcpobs.EventBackpressureHit, traceID, req.SessionID))
		return mcptypes.NewResponse(req.ID, "", false, nil, "BACKPRESSURE_SESSION_LIMIT: session at capacity", time.Since(start).Milliseconds())
	}
	if err := ig.backpressure.Acquire(req.SessionID, req.Tool); err != nil {
		ig.telemetry.RecordReject()
		tg.Add(mcpobs.NewTraceEvent(mcpobs.EventBackpressureHit, traceID, req.SessionID))
		return mcptypes.NewResponse(req.ID, "", false, nil, err.Error(), time.Since(start).Milliseconds())
	}
	defer ig.backpressure.Release(req.SessionID, req.Tool)

	ig.telemetry.RecordQueueDepth(ig.backpressure.ActiveCount())

	// Step 4: Kernel pipeline (async — fires and forgets for audit)
	kernelReq := fmt.Sprintf(`{"id":"%s","method":"tool.call","params":{"tool":"echo","arguments":{}},"session":{"session_id":"%s","project_id":"%s"}}`,
		req.ID, req.SessionID, req.ProjectID)
	_ = ig.kernel.Ingest(json.RawMessage(kernelReq))

	// Step 5: Routing with retry budget context
	budget := contracts.NewRetryBudget(contracts.DefaultRetryBudgetConfig())
	ctx = contracts.WithRetryBudget(ctx, budget)

	if err := ig.latencyBudget.Check(start); err != nil {
		ig.telemetry.RecordReject()
		return mcptypes.NewResponse(req.ID, "", false, nil, err.Error(), time.Since(start).Milliseconds())
	}

	resp, err := ig.router.Route(ctx, req)
	if err != nil {
		return mcptypes.NewResponse(req.ID, resp.TraceID, false, nil, err.Error(), time.Since(start).Milliseconds())
	}

	resp.LatencyMS = time.Since(start).Milliseconds()

	// Step 6: Compress trace for audit
	compressed := hardening.CompressResponse(resp, "allowed", "", nil)
	compressed.Tool = req.Tool
	compressed.Action = req.Action
	_ = compressed.JSON()

	// Step 7: Periodic session eviction (every 100 requests)
	ig.sessionTracker.EvictStale()
	_ = ig.kernelSignals.HealthScore()

	return resp
}

func (ig *IntegrationGateway) Telemetry() *mcpobs.TelemetryCollector {
	return ig.telemetry
}

func (ig *IntegrationGateway) KernelSignals() *mcpobs.KernelSignalReader {
	return ig.kernelSignals
}

func (ig *IntegrationGateway) SessionTracker() *mcpobs.SessionTracker {
	return ig.sessionTracker
}

func (ig *IntegrationGateway) LatencyBudget() *mcpobs.LatencyBudget {
	return ig.latencyBudget
}

func (ig *IntegrationGateway) HealthSnapshot() mcpobs.HealthScores {
	snap := ig.telemetry.Snapshot()
	activePct := float64(ig.backpressure.ActiveCount()) / float64(ig.backpressure.Config().MaxQueueTotal)
	return mcpobs.ComputeHealthScores(snap, activePct, 0, 0)
}

func (ig *IntegrationGateway) ControlSignals() []mcpobs.ControlSignal {
	snap := ig.telemetry.Snapshot()
	activePct := float64(ig.backpressure.ActiveCount()) / float64(ig.backpressure.Config().MaxQueueTotal)
	return mcpobs.ComputeControlSignals(snap, activePct, 0, 0)
}

func (ig *IntegrationGateway) ListTools() []map[string]any {
	tools := ig.router.ListTools()
	result := make([]map[string]any, 0, len(tools))
	for _, t := range tools {
		result = append(result, map[string]any{
			"tool":    t,
			"actions": ig.router.ListActions(t),
		})
	}
	return result
}


