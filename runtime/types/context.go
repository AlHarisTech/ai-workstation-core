// Package types defines the core data structures for the MCP Kernel.
// All types are immutable after construction. No methods mutate state.
package types

import "time"

type PipelineMode string

const (
	StrictMode    PipelineMode = "strict"
	OptimizedMode PipelineMode = "optimized"
)

type ContextStatus string

const (
	StatusPending  ContextStatus = "pending"
	StatusSuccess  ContextStatus = "success"
	StatusError    ContextStatus = "error"
	StatusDenied   ContextStatus = "denied"
)

type StageDecision string

const (
	DecisionAllow StageDecision = "allow"
	DecisionDeny  StageDecision = "deny"
	DecisionSkip  StageDecision = "skip"
)

type RequestContext struct {
	RequestID  string                 `json:"request_id"`
	SessionID  string                 `json:"session_id"`
	ProjectID  string                 `json:"project_id"`
	ToolID     string                 `json:"tool_id"`
	Capability string                 `json:"capability"`
	Arguments  map[string]interface{} `json:"arguments"`

	Status          ContextStatus     `json:"status"`
	ExecutionTrace  []StageResult     `json:"execution_trace"`
	DecisionPath    []string          `json:"decision_path"`
	StageTimings    map[string]float64 `json:"stage_timings"`

	WorkerID        string            `json:"worker_id"`
	QueueWaitTimeMs float64           `json:"queue_wait_time_ms"`
	PipelineMode    PipelineMode      `json:"pipeline_mode"`

	LatencyBreakdown LatencyBreakdown `json:"latency_breakdown"`
	PolicyGraph      []PolicyDecision `json:"policy_graph"`

	TimestampStart time.Time       `json:"timestamp_start"`
	TimestampEnd   time.Time       `json:"timestamp_end"`
	Result         interface{}     `json:"result"`
	Error          string          `json:"error"`
	ErrorCode      string          `json:"error_code"`
}

type LatencyBreakdown struct {
	TotalMs      float64 `json:"total_ms"`
	QueueWaitMs  float64 `json:"queue_wait_ms"`
	RoutingMs    float64 `json:"routing_ms"`
	ExecutionMs  float64 `json:"execution_ms"`
	AuditMs      float64 `json:"audit_ms"`
	ValidationMs float64 `json:"validation_ms"`
}

type StageResult struct {
	Stage      string                 `json:"stage"`
	Decision   StageDecision          `json:"decision"`
	DurationMs float64                `json:"duration_ms"`
	Detail     map[string]interface{} `json:"detail"`
	Error      string                 `json:"error"`
	Timestamp  time.Time              `json:"timestamp"`
}

type PolicyDecision struct {
	RuleID   string `json:"rule_id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Stage    string `json:"stage"`
	Priority int    `json:"priority"`
}

type PolicyVerdict struct {
	Decision  string           `json:"decision"`
	Reason    string           `json:"reason"`
	RuleID    string           `json:"rule_id"`
	RuleChain []PolicyDecision `json:"rule_chain"`
	Timestamp time.Time        `json:"timestamp"`
}

func NewRequestContext(requestID, sessionID, projectID string, toolID string, args map[string]interface{}, mode PipelineMode) *RequestContext {
	if mode != StrictMode && mode != OptimizedMode {
		mode = StrictMode
	}
	return &RequestContext{
		RequestID:      requestID,
		SessionID:      sessionID,
		ProjectID:      projectID,
		ToolID:         toolID,
		Arguments:      args,
		Status:         StatusPending,
		StageTimings:   make(map[string]float64),
		ExecutionTrace: make([]StageResult, 0, 8),
		DecisionPath:   make([]string, 0, 8),
		PolicyGraph:    make([]PolicyDecision, 0),
		PipelineMode:   mode,
		TimestampStart: time.Now(),
	}
}

func (rc *RequestContext) AppendTrace(sr StageResult) {
	rc.ExecutionTrace = append(rc.ExecutionTrace, sr)
	rc.DecisionPath = append(rc.DecisionPath, sr.Stage)
	rc.StageTimings[sr.Stage] = sr.DurationMs
}

func (rc *RequestContext) Finalize(status ContextStatus, err, errCode string) {
	rc.Status = status
	rc.Error = err
	rc.ErrorCode = errCode
	rc.TimestampEnd = time.Now()
}

func (rc *RequestContext) WasDenied() bool {
	for _, tr := range rc.ExecutionTrace {
		if tr.Decision == DecisionDeny {
			return true
		}
	}
	return false
}
