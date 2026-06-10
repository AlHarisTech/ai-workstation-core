package observability

import "time"

type EventType string

const (
	EventGatewayRequest    EventType = "gateway.request"
	EventGatewayReject     EventType = "gateway.reject"
	EventRouterResolve     EventType = "router.resolve"
	EventRouterError       EventType = "router.error"
	EventAdapterExecute    EventType = "adapter.execute"
	EventAdapterComplete   EventType = "adapter.complete"
	EventAdapterRetry      EventType = "adapter.retry"
	EventAdapterFail       EventType = "adapter.fail"
	EventCircuitState      EventType = "circuit.state"
	EventCircuitReject     EventType = "circuit.reject"
	EventToolStart         EventType = "tool.start"
	EventToolComplete      EventType = "tool.complete"
	EventToolError         EventType = "tool.error"
	EventBackpressureHit    EventType = "backpressure.hit"
	EventRetryBudgetExhaust EventType = "retry.budget_exhaust"
	EventReleaseStart       EventType = "release.start"
	EventReleaseTagCreated  EventType = "release.tag_created"
	EventReleasePublished        EventType = "release.published"
	EventReleaseFailure          EventType = "release.failure"
	EventReleasePendingExternal  EventType = "release.pending_external"
	EventReleaseQueued           EventType = "release.queued"
	EventReleaseRetryScheduled   EventType = "release.retry_scheduled"
	EventReleaseExternalRecovered EventType = "release.external_recovered"
	EventReleaseFinalFailure     EventType = "release.final_failure"
)

type TraceEvent struct {
	Type      EventType `json:"type"`
	Timestamp int64     `json:"ts"`
	TraceID   string    `json:"trace_id"`
	SessionID string    `json:"session_id"`
	Tool      string    `json:"tool,omitempty"`
	Action    string    `json:"action,omitempty"`
	Status    string    `json:"status,omitempty"`
	LatencyMS int64     `json:"latency_ms,omitempty"`
	Error     string    `json:"error,omitempty"`
	Detail    string    `json:"detail,omitempty"`
}

func NewTraceEvent(typ EventType, traceID, sessionID string) TraceEvent {
	return TraceEvent{
		Type:      typ,
		Timestamp: time.Now().UnixMilli(),
		TraceID:   traceID,
		SessionID: sessionID,
	}
}

type ToolEvent struct {
	TraceID    string `json:"trace_id"`
	Tool       string `json:"tool"`
	Action     string `json:"action"`
	Success    bool   `json:"success"`
	LatencyMS  int64  `json:"latency_ms"`
	RetryCount int    `json:"retry_count,omitempty"`
	CircuitState string `json:"circuit_state,omitempty"`
	Error      string `json:"error,omitempty"`
	Timestamp  int64  `json:"ts"`
}

type RoutingEvent struct {
	TraceID   string `json:"trace_id"`
	Tool      string `json:"tool"`
	Action    string `json:"action"`
	Resolved  bool   `json:"resolved"`
	Adapter   string `json:"adapter,omitempty"`
	LatencyMS int64  `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
	Timestamp int64  `json:"ts"`
}

type FailureEvent struct {
	TraceID   string `json:"trace_id"`
	Component string `json:"component"`
	Tool      string `json:"tool,omitempty"`
	Action    string `json:"action,omitempty"`
	Error     string `json:"error"`
	Stage     string `json:"stage"`
	LatencyMS int64  `json:"latency_ms"`
	Timestamp int64  `json:"ts"`
}

type CircuitEvent struct {
	TraceID   string `json:"trace_id"`
	Tool      string `json:"tool"`
	OldState  string `json:"old_state"`
	NewState  string `json:"new_state"`
	FailCount int    `json:"fail_count"`
	Timestamp int64  `json:"ts"`
}
