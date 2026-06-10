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

type HardeningConfig struct {
	HeartbeatInterval time.Duration
	SnapshotInterval  time.Duration
	MaxRestartCount   int
	FairQueueEnabled  bool
	CompactionMaxAge  time.Duration
}

func DefaultHardeningConfig() HardeningConfig {
	return HardeningConfig{
		HeartbeatInterval: 5 * time.Second,
		SnapshotInterval:  60 * time.Second,
		MaxRestartCount:   3,
		FairQueueEnabled:  false,
		CompactionMaxAge:  24 * time.Hour,
	}
}

type KernelEngine struct {
	config         KernelConfig
	harden         HardeningConfig
	queue          *queue.RequestQueue
	fairQueue      *queue.FairQueue
	pool           *worker.WorkerPool
	policy         *policy.PolicyEngine
	executor       *executor.TimeoutWrapper
	stateStore     *state.StateStore
	logger         *observability.StructuredLogger
	tracer         *observability.Tracer
	metrics        *observability.KernelMetrics
	latency        *observability.LatencyTracker
	loadShedder    *LoadShedder
	consistency    *state.ConsistencyGuard
	lifecycle      *Lifecycle

	resultChan     chan *types.RequestContext
	errorChan      chan types.KernelEvent
	stateWriteChan chan *types.RequestContext

	ingestStopped   bool
	shutdownStarted time.Time
}

func NewKernelEngine(cfg KernelConfig, harden HardeningConfig) (*KernelEngine, error) {
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
	resultChan := make(chan *types.RequestContext, cfg.QueueSize)
	errorChan := make(chan types.KernelEvent, 64)
	stateWriteChan := make(chan *types.RequestContext, cfg.QueueSize)

	policyEngine := policy.NewPolicyEngine(policy.DefaultRules(), "0.5.0")
	reg := &SimpleRegistry{}
	execCore := executor.NewExecutionCore(reg)
	timeoutWrapper := executor.NewTimeoutWrapper(execCore, 30*time.Second)

	pool := worker.NewWorkerPool(
		cfg.WorkerCount,
		reqQueue.Chan(),
		resultChan,
		errorChan,
		policyEngine,
		timeoutWrapper,
		observability.LogFn(logger),
		reg,
	)

	return &KernelEngine{
		config:         cfg,
		harden:         harden,
		queue:          reqQueue,
		fairQueue:      queue.NewFairQueue(cfg.QueueSize),
		pool:           pool,
		policy:         policyEngine,
		executor:       timeoutWrapper,
		stateStore:     state.NewStateStore(statePath),
		logger:         logger,
		tracer:         observability.NewTracer(),
		metrics:        observability.NewKernelMetrics(),
		latency:        observability.NewLatencyTracker(2000),
		loadShedder:    NewLoadShedder(),
		consistency:    state.NewConsistencyGuard(),
		lifecycle:      NewLifecycle(),
		resultChan:     resultChan,
		errorChan:      errorChan,
		stateWriteChan: stateWriteChan,
	}, nil
}

func (ke *KernelEngine) Start() {
	ke.pool.Start(ke.lifecycle.Context())
	ke.lifecycle.WaitForSignal()

	ke.logger.Log(types.LogEntry{
		Status:    "kernel_started",
		Timestamp: time.Now(),
	})

	go ke.collectResults()
	go ke.collectErrors()
	go ke.stateWriter()
	go ke.metricsEmitter()
	if ke.harden.SnapshotInterval > 0 {
		go ke.snapshotGenerator()
	}
}

func (ke *KernelEngine) Ingest(raw json.RawMessage) error {
	if ke.ingestStopped {
		return fmt.Errorf("INGEST_STOPPED: shutdown in progress")
	}

	ke.metrics.Ingressed.Add(1)

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
		ke.metrics.Rejected.Add(1)
		ke.metrics.QueueDepth.Add(-1)
		ke.tracer.TraceQueueReject(ctx.RequestID)
		return err
	}

	ke.metrics.Enqueued.Add(1)
	return nil
}

func (ke *KernelEngine) GracefulShutdown() {
	ke.shutdownStarted = time.Now()

	ke.logger.Log(types.LogEntry{
		Status:    "shutdown_phase_1_stop_ingestion",
		Timestamp: time.Now(),
	})
	ke.ingestStopped = true

	ke.lifecycle.GracefulShutdown()

	ke.logger.Log(types.LogEntry{
		Status:    "shutdown_phase_2_drain_queue",
		Timestamp: time.Now(),
	})
	drainWait := 500 * time.Millisecond
	deadline := time.Now().Add(drainWait)
	for ke.queue.Size() > 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	ke.queue.Close()

	ke.logger.Log(types.LogEntry{
		Status:    "shutdown_phase_3_wait_workers",
		Timestamp: time.Now(),
	})
	ke.pool.Wait()

	ke.logger.Log(types.LogEntry{
		Status:   "shutdown_phase_4_flush_state",
		Timestamp: time.Now(),
	})
	close(ke.resultChan)
	close(ke.errorChan)
	close(ke.stateWriteChan)

	_, _ = ke.stateStore.GenerateSnapshot(
		ke.pool.Supervisor().AllHealth(),
		ke.metrics.Snapshot(),
	)

	ke.logger.Log(types.LogEntry{
		Status:    "shutdown_snapshot",
		Timestamp: time.Now(),
	})

	ke.logger.Log(types.LogEntry{
		Status:    "shutdown_phase_5_complete",
		Timestamp: time.Now(),
	})
	ke.logger.Close()
}

func (ke *KernelEngine) Shutdown() {
	ke.GracefulShutdown()
}

func (ke *KernelEngine) collectResults() {
	for ctx := range ke.resultChan {
		ke.metrics.QueueDepth.Add(-1)

		totalMs := 0.0
		for _, t := range ctx.StageTimings {
			totalMs += t
		}
		ke.latency.Record(totalMs)
		sla := ke.latency.Snapshot()
		if sla.Violation {
			ke.loadShedder.RecordSLAViolation()
		}

		switch ctx.Status {
		case types.StatusSuccess:
			ke.metrics.Completed.Add(1)
		case types.StatusDenied:
			ke.metrics.Denied.Add(1)
		case types.StatusError:
			ke.metrics.Failed.Add(1)
		}
		response, _ := json.Marshal(ctx)
		fmt.Println(string(response))

		ke.metrics.Cycles.Add(1)
		ke.stateWriteChan <- ctx
	}
}

func (ke *KernelEngine) stateWriter() {
	for ctx := range ke.stateWriteChan {
		if err := ke.stateStore.SaveTraceWithRetry(ctx); err != nil {
			ke.logger.Log(types.LogEntry{
				RequestID: ctx.RequestID,
				Status:    "STATE_WRITE_FAILED",
				Error:     err.Error(),
				Timestamp: time.Now(),
			})
		}
	}
}

func (ke *KernelEngine) metricsEmitter() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ke.lifecycle.Context().Done():
			return
		case <-ticker.C:
			ke.emitMetricsCycle()
		}
	}
}

func (ke *KernelEngine) emitMetricsCycle() {
	health := ke.pool.Supervisor().FailureSummary()
	snap := ke.metrics.Snapshot()
	sla := ke.latency.Snapshot()
	loadState := ke.loadShedder.State()
	utilization := ke.pool.Supervisor().Utilization()
	maxStarv := ke.fairQueue.MaxStarvationMs()

	ke.loadShedder.Evaluate(ke.queue.Size(), ke.queue.MaxSize(), utilization)

	entry := types.LogEntry{
		Status:    "metrics_cycle",
		Timestamp: time.Now(),
	}
	payload := map[string]interface{}{
		"metrics":          snap,
		"sla":              sla,
		"worker_health":    health,
		"worker_utilization": utilization,
		"load_state":       loadState.String(),
		"queue_depth":      ke.queue.Size(),
		"queue_max":        ke.queue.MaxSize(),
		"queue_saturation": float64(ke.queue.Size()) / float64(ke.queue.MaxSize()),
		"max_starvation_ms": maxStarv,
		"sla_violations":   ke.loadShedder.SLAViolations(),
	}
	data, _ := json.Marshal(payload)
	entry.Error = string(data)
	ke.logger.Log(entry)
}

func (ke *KernelEngine) snapshotGenerator() {
	ticker := time.NewTicker(ke.harden.SnapshotInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ke.lifecycle.Context().Done():
			ke.stateStore.GenerateSnapshot(
				ke.pool.Supervisor().AllHealth(),
				ke.metrics.Snapshot(),
			)
			return
		case <-ticker.C:
			ke.stateStore.GenerateSnapshot(
				ke.pool.Supervisor().AllHealth(),
				ke.metrics.Snapshot(),
			)
		}
	}
}

func (ke *KernelEngine) collectErrors() {
	for evt := range ke.errorChan {
		ke.tracer.TraceEvent(evt)
	}
}

func (ke *KernelEngine) ReplayHistory() (*state.ReplayResult, error) {
	return ke.stateStore.ReplayHistory()
}

func (ke *KernelEngine) CompactTraces(olderThan time.Duration) (int, error) {
	return ke.stateStore.CompactTraces(olderThan)
}

func (ke *KernelEngine) Supervisor() *worker.WorkerSupervisor {
	return ke.pool.Supervisor()
}

func (ke *KernelEngine) Queue() *queue.RequestQueue  { return ke.queue }
func (ke *KernelEngine) Metrics() *observability.KernelMetrics { return ke.metrics }
func (ke *KernelEngine) Tracer() *observability.Tracer        { return ke.tracer }
