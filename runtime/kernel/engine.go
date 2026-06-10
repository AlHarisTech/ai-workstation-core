package kernel

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/executor"
	"github.com/AlHarisTech/ai-workstation-core/runtime/observability"
	"github.com/AlHarisTech/ai-workstation-core/runtime/policy"
	"github.com/AlHarisTech/ai-workstation-core/runtime/queue"
	"github.com/AlHarisTech/ai-workstation-core/runtime/state"
	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
	"github.com/AlHarisTech/ai-workstation-core/runtime/worker"
)

type KernelEngine struct {
	config   KernelConfig
	queue    *queue.RequestQueue
	pool     *worker.WorkerPool
	policy   *policy.PolicyEngine
	executor *executor.TimeoutWrapper
	state    *state.StateStore
	logger   *observability.StructuredLogger
	tracer   *observability.Tracer
	metrics  *observability.Metrics
	lifecycle *Lifecycle
	results  chan *types.RequestContext
	errors   chan types.KernelEvent
}

type SimpleRegistry struct{}

func (sr *SimpleRegistry) GetTool(toolID string) (*types.ToolDef, error) {
	tools := map[string]*types.ToolDef{
		"echo": {
			ID: "echo", Name: "Echo", Type: "built-in", Version: "1.0",
			Capabilities: []string{"echo"},
			Governance:   types.ToolGovernance{RequireSession: false},
		},
		"filesystem_read": {
			ID: "filesystem_read", Name: "Filesystem Reader", Type: "built-in", Version: "1.0",
			Capabilities: []string{"file-read"},
			Governance:   types.ToolGovernance{RequireSession: true, TimeoutMs: 5000},
		},
	}
	if tool, ok := tools[toolID]; ok {
		return tool, nil
	}
	return nil, fmt.Errorf("tool not found: %s", toolID)
}

func NewKernelEngine(cfg KernelConfig) (*KernelEngine, error) {
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4
	}
	if cfg.QueueSize <= 0 {
		cfg.QueueSize = 128
	}

	logPath := cfg.LogPath
	if !os.IsPathSeparator(logPath[0]) {
		logPath = cfg.WorkspaceRoot + "/" + logPath
	}
	statePath := cfg.StatePath
	if !os.IsPathSeparator(statePath[0]) {
		statePath = cfg.WorkspaceRoot + "/" + statePath
	}

	logger, err := observability.NewStructuredLogger(logPath)
	if err != nil {
		return nil, fmt.Errorf("logger init: %w", err)
	}

	reqQueue := queue.NewRequestQueue(cfg.QueueSize)
	results := make(chan *types.RequestContext, cfg.QueueSize)
	errs := make(chan types.KernelEvent, 64)

	policyEngine := policy.NewPolicyEngine(policy.DefaultRules(), "0.4.1")
	reg := &SimpleRegistry{}
	execCore := executor.NewExecutionCore(reg)
	timeoutWrapper := executor.NewTimeoutWrapper(execCore, 30*time.Second)

	pool := worker.NewWorkerPool(
		cfg.WorkerCount,
		reqQueue.Chan(),
		results,
		errs,
		policyEngine,
		timeoutWrapper,
		observability.LogFn(logger),
		reg,
	)

	tracer := observability.NewTracer()
	metrics := observability.NewMetrics()
	lifecycle := NewLifecycle()

	return &KernelEngine{
		config:    cfg,
		queue:     reqQueue,
		pool:      pool,
		policy:    policyEngine,
		executor:  timeoutWrapper,
		state:     state.NewStateStore(statePath),
		logger:    logger,
		tracer:    tracer,
		metrics:   metrics,
		lifecycle: lifecycle,
		results:   results,
		errors:    errs,
	}, nil
}

func (ke *KernelEngine) Start() {
	ke.pool.Start(ke.lifecycle.Context())
	ke.lifecycle.WaitForSignal()

	ke.logger.Log(types.LogEntry{
		Status: "kernel_started",
		Timestamp: time.Now(),
	})

	go ke.collectResults()
	go ke.collectErrors()
}

func (ke *KernelEngine) Ingest(raw json.RawMessage) error {
	ke.metrics.RequestsIngressed.Add(1)
	ke.tracer.TraceRequestIngress("ingress")

	var req types.RawRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return fmt.Errorf("PARSE_ERROR: %w", err)
	}

	pipelineMode := types.StrictMode
	if req.PipelineMode == "optimized" {
		pipelineMode = types.OptimizedMode
	}

	requestID := req.ID
	if requestID == "" {
		requestID = fmt.Sprintf("auto_%d", time.Now().UnixNano())
	}

	ctx := types.NewRequestContext(
		requestID,
		req.Session.SessionID,
		req.Session.ProjectID,
		req.Params.Tool,
		req.Params.Arguments,
		pipelineMode,
	)
	ctx.TimestampStart = time.Now()

	ke.metrics.QueueDepth.Add(1)
	ke.tracer.TraceQueueEnqueue(ctx.RequestID)

	if err := ke.queue.Enqueue(ctx); err != nil {
		ke.metrics.RequestsRejected.Add(1)
		ke.metrics.QueueDepth.Add(-1)
		ke.tracer.TraceQueueReject(ctx.RequestID)
		return err
	}

	ke.metrics.RequestsEnqueued.Add(1)
	return nil
}

func (ke *KernelEngine) Shutdown() {
	ke.lifecycle.GracefulShutdown()
	time.Sleep(300 * time.Millisecond)
	ke.queue.Close()
	ke.pool.Wait()
	close(ke.results)
	close(ke.errors)
	ke.logger.Log(types.LogEntry{
		Status:    "kernel_stopped",
		Timestamp: time.Now(),
	})
	ke.logger.Close()
}

func (ke *KernelEngine) collectResults() {
	for ctx := range ke.results {
		ke.metrics.QueueDepth.Add(-1)
		if ctx.Status == types.StatusSuccess {
			ke.metrics.RequestsCompleted.Add(1)
		} else if ctx.Status == types.StatusError {
			ke.metrics.RequestsDenied.Add(1)
			ke.metrics.ExecutionsFailed.Add(1)
		} else {
			ke.metrics.RequestsDenied.Add(1)
		}
		if err := ke.state.SaveTraceWithRetry(ctx); err != nil {
			ke.logger.Log(types.LogEntry{
				RequestID: ctx.RequestID,
				Status:    "STATE_WRITE_FAILED",
				Error:     err.Error(),
				Timestamp: time.Now(),
			})
		}
		response, _ := json.Marshal(ctx)
		fmt.Println(string(response))
	}
}

func (ke *KernelEngine) collectErrors() {
	for evt := range ke.errors {
		ke.tracer.TraceEvent(evt)
	}
}

func (ke *KernelEngine) Queue() *queue.RequestQueue { return ke.queue }
func (ke *KernelEngine) Metrics() *observability.Metrics { return ke.metrics }
func (ke *KernelEngine) Tracer() *observability.Tracer { return ke.tracer }
