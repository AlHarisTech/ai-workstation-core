package types

import "time"

type RawRequest struct {
	ID           string            `json:"id"`
	Method       string            `json:"method"`
	PipelineMode string            `json:"pipeline_mode,omitempty"`
	Params       RequestParams     `json:"params"`
	Session      SessionData       `json:"session"`
}

type RequestParams struct {
	Tool       string                 `json:"tool,omitempty"`
	Capability string                 `json:"capability,omitempty"`
	Arguments  map[string]interface{} `json:"arguments,omitempty"`
}

type SessionData struct {
	SessionID string `json:"session_id,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
	UserID    string `json:"user_id,omitempty"`
}

type ToolDef struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Type         string            `json:"type"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Capabilities []string          `json:"capabilities"`
	Governance   ToolGovernance    `json:"governance"`
	Protocol     string            `json:"protocol"`
}

type ToolGovernance struct {
	RequireSession bool `json:"require_session"`
	AuditLevel     string `json:"audit_level"`
	TimeoutMs      int    `json:"timeout_ms,omitempty"`
}

type ExecResult struct {
	Status     string      `json:"status"`
	Result     interface{} `json:"result"`
	Error      string      `json:"error"`
	ErrorCode  string      `json:"error_code"`
	ErrorTrace string      `json:"error_trace,omitempty"`
	DurationMs float64     `json:"duration_ms"`
}

type LogEntry struct {
	RequestID         string            `json:"request_id"`
	SessionID         string            `json:"session_id"`
	ProjectID         string            `json:"project_id"`
	ToolID            string            `json:"tool_id"`
	Status            string            `json:"status"`
	DecisionPath      []string          `json:"decision_path"`
	StageTimings      map[string]float64 `json:"stage_timings"`
	TotalMs           float64           `json:"total_ms"`
	QueueWaitTimeMs   float64           `json:"queue_wait_time_ms"`
	WorkerID          string            `json:"worker_id"`
	PipelineMode      string            `json:"pipeline_mode"`
	LatencyBreakdown  LatencyBreakdown  `json:"latency_breakdown"`
	PolicyGraph       []PolicyDecision  `json:"policy_decision_graph"`
	ExecutionTrace    []StageResult     `json:"execution_trace"`
	Error             string            `json:"error"`
	ErrorCode         string            `json:"error_code"`
	Timestamp         time.Time         `json:"timestamp"`
}
