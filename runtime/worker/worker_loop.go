package worker

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/executor"
	"github.com/AlHarisTech/ai-workstation-core/runtime/policy"
	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type Worker struct {
	ID         string
	queue      <-chan *types.RequestContext
	results    chan<- *types.RequestContext
	errors     chan<- types.KernelEvent
	policy     *policy.PolicyEngine
	executor   *executor.TimeoutWrapper
	logger     func(types.LogEntry)
	pipeline   *Pipeline
	supervisor *WorkerSupervisor
	health     *WorkerHealth
	heartbeat  time.Duration
}

func (w *Worker) Run(ctx context.Context) {
	w.health.MarkAlive()
	ticker := time.NewTicker(w.heartbeat)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.health.MarkAlive()
		case reqCtx, ok := <-w.queue:
			if !ok {
				return
			}
			w.processOne(ctx, reqCtx)
			w.health.IncrementRequests()
		}
	}
}

func (w *Worker) processOne(ctx context.Context, reqCtx *types.RequestContext) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			log.Printf("[WORKER_PANIC] worker=%s request=%s panic=%v\n%s", w.ID, reqCtx.RequestID, r, string(stack))
			w.health.MarkDegraded(fmt.Sprintf("panic: %v", r))
			reqCtx.Finalize(types.StatusError, fmt.Sprintf("worker panic: %v", r), "WORKER_PANIC")
			w.results <- reqCtx
		}
	}()

	reqCtx.WorkerID = w.ID
	dequeueTime := time.Now()
	reqCtx.QueueWaitTimeMs = float64(dequeueTime.Sub(reqCtx.TimestampStart).Microseconds()) / 1000.0

	// Run pipeline stages (simplified for Go kernel)
	result := w.runStages(ctx, reqCtx)

	w.results <- result
}

func (w *Worker) runStages(ctx context.Context, reqCtx *types.RequestContext) *types.RequestContext {
	stages := []struct {
		name    string
		handler func(*types.RequestContext, *types.ToolDef) (types.StageDecision, string)
	}{
		{"pre_validation", w.stagePreValidation},
		{"session_guard", w.stageSessionGuard},
		{"capability_routing", w.stageCapabilityRouting},
		{"pre_execution", w.stagePreExecution},
		{"execution", w.stageExecution},
		{"post_validation", w.stagePostValidation},
		{"audit_log", w.stageAuditLog},
	}

	var toolDef *types.ToolDef
	if w.pipeline != nil && w.pipeline.registry != nil {
		if td, err := w.pipeline.registry.GetTool(reqCtx.ToolID); err == nil {
			toolDef = td
		}
	}

	for _, stage := range stages {
		t0 := time.Now()
		decision, errStr := stage.handler(reqCtx, toolDef)
		reqCtx.AppendTrace(types.StageResult{
			Stage:      stage.name,
			Decision:   decision,
			DurationMs: float64(time.Since(t0).Microseconds()) / 1000.0,
			Error:      errStr,
			Timestamp:  time.Now(),
		})
		if decision == types.DecisionDeny {
			reqCtx.Finalize(types.StatusDenied, errStr, "DENIED_AT_"+stage.name)
			w.computeLatency(reqCtx)
			return reqCtx
		}
	}

	reqCtx.Finalize(types.StatusSuccess, "", "")
	w.computeLatency(reqCtx)
	return reqCtx
}

func (w *Worker) stagePreValidation(reqCtx *types.RequestContext, _ *types.ToolDef) (types.StageDecision, string) {
	verdict := w.policy.EvaluateStage("pre_validation", reqCtx, nil)
	if verdict.Decision == "DENY" {
		return types.DecisionDeny, verdict.Reason
	}
	return types.DecisionAllow, ""
}

func (w *Worker) stageSessionGuard(reqCtx *types.RequestContext, toolDef *types.ToolDef) (types.StageDecision, string) {
	verdict := w.policy.EvaluateStage("session_guard", reqCtx, toolDef)
	if verdict.Decision == "DENY" {
		return types.DecisionDeny, verdict.Reason
	}
	return types.DecisionAllow, ""
}

func (w *Worker) stageCapabilityRouting(reqCtx *types.RequestContext, toolDef *types.ToolDef) (types.StageDecision, string) {
	if toolDef == nil && reqCtx.ToolID != "" {
		return types.DecisionDeny, "tool not found: " + reqCtx.ToolID
	}
	return types.DecisionAllow, ""
}

func (w *Worker) stagePreExecution(reqCtx *types.RequestContext, _ *types.ToolDef) (types.StageDecision, string) {
	verdict := w.policy.EvaluateStage("pre_execution", reqCtx, nil)
	if verdict.Decision == "DENY" {
		return types.DecisionDeny, verdict.Reason
	}
	return types.DecisionAllow, ""
}

func (w *Worker) stageExecution(reqCtx *types.RequestContext, _ *types.ToolDef) (types.StageDecision, string) {
	result := w.executor.ExecuteIsolated(context.Background(), reqCtx.ToolID, reqCtx.Arguments)
	if result.Status == "error" {
		reqCtx.Status = types.StatusError
		reqCtx.Error = result.Error
		reqCtx.ErrorCode = result.ErrorCode
		return types.DecisionDeny, result.Error
	}
	reqCtx.Status = types.StatusSuccess
	reqCtx.Result = result.Result
	return types.DecisionAllow, ""
}

func (w *Worker) stagePostValidation(reqCtx *types.RequestContext, _ *types.ToolDef) (types.StageDecision, string) {
	if reqCtx.Status == types.StatusError {
		verdict := w.policy.EvaluateStage("post_validation", reqCtx, nil)
		if verdict.Decision == "DENY" {
			return types.DecisionDeny, "execution error contained"
		}
	}
	return types.DecisionAllow, ""
}

func (w *Worker) stageAuditLog(reqCtx *types.RequestContext, _ *types.ToolDef) (types.StageDecision, string) {
	if w.logger != nil {
		w.logger(types.LogEntry{
			RequestID:        reqCtx.RequestID,
			SessionID:        reqCtx.SessionID,
			ProjectID:        reqCtx.ProjectID,
			ToolID:           reqCtx.ToolID,
			Status:           string(reqCtx.Status),
			DecisionPath:     reqCtx.DecisionPath,
			StageTimings:     reqCtx.StageTimings,
			QueueWaitTimeMs:  reqCtx.QueueWaitTimeMs,
			WorkerID:         w.ID,
			PipelineMode:     string(reqCtx.PipelineMode),
			LatencyBreakdown: reqCtx.LatencyBreakdown,
			PolicyGraph:      reqCtx.PolicyGraph,
			ExecutionTrace:   reqCtx.ExecutionTrace,
			Error:            reqCtx.Error,
			ErrorCode:        reqCtx.ErrorCode,
			Timestamp:        time.Now(),
		})
	}
	return types.DecisionAllow, ""
}

func (w *Worker) computeLatency(reqCtx *types.RequestContext) {
	total := 0.0
	for _, t := range reqCtx.StageTimings {
		total += t
	}
	reqCtx.LatencyBreakdown = types.LatencyBreakdown{
		TotalMs:      total,
		QueueWaitMs:  reqCtx.QueueWaitTimeMs,
		RoutingMs:    reqCtx.StageTimings["capability_routing"],
		ExecutionMs:  reqCtx.StageTimings["execution"],
		AuditMs:      reqCtx.StageTimings["audit_log"],
		ValidationMs: reqCtx.StageTimings["pre_validation"] + reqCtx.StageTimings["post_validation"],
	}
}

type Pipeline struct {
	registry ToolRegistry
}

type ToolRegistry interface {
	GetTool(toolID string) (*types.ToolDef, error)
}

func NewPipeline(registry ToolRegistry) *Pipeline {
	return &Pipeline{registry: registry}
}
