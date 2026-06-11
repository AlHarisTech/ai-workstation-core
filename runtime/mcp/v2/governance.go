package mcpv2

import (
	"encoding/json"
	"log"
)

type AuditRecord struct {
	Timestamp        string `json:"timestamp"`
	RequestID        string `json:"request_id"`
	TraceID          string `json:"trace_id"`
	Action           string `json:"action"`
	Server           string `json:"server"`
	Status           string `json:"status"`
	DurationMs       int64  `json:"duration_ms"`
	KnowledgeCount   int    `json:"knowledge_count"`
	ExecutionAllowed bool   `json:"execution_allowed"`
	BlockReason      string `json:"block_reason,omitempty"`
}

func LogAudit(r AuditRecord) {
	data, _ := json.Marshal(r)
	log.Printf("[audit] %s", string(data))
}
