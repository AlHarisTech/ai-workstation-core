package orchestratorv1

type OrchestratorEvent struct {
	TraceID   string         `json:"trace_id"`
	Source    string         `json:"source"`
	EventType string         `json:"event_type"`
	Execution EventExecution `json:"execution"`
	Context   EventContext   `json:"context"`
}

type EventExecution struct {
	Server    string `json:"server"`
	Operation string `json:"operation"`
	Result    any    `json:"result"`
}

type EventContext struct {
	SessionID string `json:"session_id"`
	TenantID  string `json:"tenant_id"`
}

type OrchestratorResponse struct {
	TraceID          string   `json:"trace_id"`
	Status           string   `json:"status"`
	SystemsTriggered []string `json:"systems_triggered"`
	Result           any      `json:"result"`
}

type RoutingDecision struct {
	Postgres  bool
	ChromaDB  bool
	LangGraph bool
	CrewAI    bool
}
