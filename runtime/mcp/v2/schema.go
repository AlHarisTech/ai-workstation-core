package mcpv2

type ActionType string

const (
	ActionGit        ActionType = "git"
	ActionFilesystem ActionType = "filesystem"
	ActionMemory     ActionType = "memory"
	ActionGitHub     ActionType = "github"
	ActionFetch      ActionType = "fetch"
	ActionContext7   ActionType = "context7"
	ActionPostgres   ActionType = "postgres"
	ActionChromaDB   ActionType = "chroma"
)

type MCPAction struct {
	Type      ActionType `json:"type"`
	Operation string     `json:"operation"`
	Version   string     `json:"version"`
}

type KnowledgeDoc struct {
	Collection string `json:"collection"`
	Query      string `json:"query"`
	Results    any    `json:"results"`
	DurationMs int64  `json:"duration_ms"`
}

type MCPContext struct {
	TenantID   string          `json:"tenant_id"`
	SessionID  string          `json:"session_id"`
	TraceID    string          `json:"trace_id"`
	TimeoutMs  int             `json:"timeout_ms"`
	Knowledge  []KnowledgeDoc  `json:"knowledge,omitempty"`
	Workspace struct {
		Path string `json:"path"`
		Repo string `json:"repo"`
	} `json:"workspace"`
}

type MCPAuth struct {
	Type  string   `json:"type"`
	Scope []string `json:"scope"`
}

type MCPPolicy struct {
	Allow []string `json:"allow"`
	Deny  []string `json:"deny"`
}

type MCPMeta struct {
	Priority string `json:"priority"`
	Timeout  int    `json:"timeout_ms"`
	Retry    int    `json:"retry"`
	TraceID  string `json:"trace_id"`
	SpanID   string `json:"span_id"`
}

type MCPRequest struct {
	ID        string     `json:"id"`
	Timestamp string     `json:"timestamp"`
	Source    string     `json:"source"`
	Type      string     `json:"type"`
	Action    MCPAction  `json:"action"`
	Context   MCPContext `json:"context"`
	Payload   struct {
		Resource   string         `json:"resource"`
		Parameters map[string]any `json:"parameters"`
	} `json:"payload"`
	Auth   MCPAuth   `json:"auth"`
	Policy MCPPolicy `json:"policy"`
	Meta   MCPMeta   `json:"meta"`
}

type ExecutionResult struct {
	Server    string `json:"server"`
	Operation string `json:"operation"`
	Duration  int64  `json:"duration_ms"`
}

type ResultData struct {
	Data   any    `json:"data"`
	Format string `json:"format"`
}

type ErrorInfo struct {
	Code        string `json:"code"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
}

type ResponseMeta struct {
	Cached        bool            `json:"cached"`
	Retried       int             `json:"retried"`
	TraceID       string          `json:"trace_id"`
	SpanID        string          `json:"span_id"`
	DecisionTrace *DecisionTrace  `json:"decision_trace,omitempty"`
}

type TraceStep struct {
	Stage  string         `json:"stage"`
	Input  string         `json:"input,omitempty"`
	Output string         `json:"output,omitempty"`
	Meta   map[string]any `json:"meta,omitempty"`
}

type DecisionTrace struct {
	TraceID        string                `json:"trace_id"`
	RequestID      string                `json:"request_id"`
	ServerScores   map[string]float64    `json:"server_scores,omitempty"`
	SelectedServer string                `json:"selected_server"`
	SecondBest     string                `json:"second_best,omitempty"`
	ScoreDelta     float64               `json:"score_delta,omitempty"`
	KnowledgeUsed  []string              `json:"knowledge_used,omitempty"`
	Steps          []TraceStep           `json:"steps"`
}

type MCPResponse struct {
	ID        string          `json:"id"`
	RequestID string          `json:"request_id"`
	Status    string          `json:"status"`
	Execution ExecutionResult `json:"execution"`
	Result    ResultData      `json:"result"`
	Error     ErrorInfo       `json:"error"`
	Meta      ResponseMeta    `json:"meta"`
}
