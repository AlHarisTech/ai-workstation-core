package types

import (
	"context"
	"time"
)

type MCPRequest struct {
	ID        string         `json:"id"`
	SessionID string         `json:"session_id"`
	ProjectID string         `json:"project_id"`
	Tool      string         `json:"tool"`
	Action    string         `json:"action"`
	Payload   map[string]any `json:"payload"`
	Timestamp int64          `json:"timestamp"`
}

type MCPResponse struct {
	ID        string `json:"id"`
	Success   bool   `json:"success"`
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	TraceID   string `json:"trace_id"`
	LatencyMS int64  `json:"latency_ms"`
	Timestamp int64  `json:"timestamp"`
}

type MCPAdapter interface {
	Name() string
	Execute(ctx context.Context, req MCPRequest) (MCPResponse, error)
}

type RouteEntry struct {
	Tool    string
	Action  string
	Adapter MCPAdapter
}

func NewRequest(id, sessionID, projectID, tool, action string, payload map[string]any) MCPRequest {
	return MCPRequest{
		ID:        id,
		SessionID: sessionID,
		ProjectID: projectID,
		Tool:      tool,
		Action:    action,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}
}

func NewResponse(id, traceID string, success bool, data any, err string, latencyMS int64) MCPResponse {
	return MCPResponse{
		ID:        id,
		Success:   success,
		Data:      data,
		Error:     err,
		TraceID:   traceID,
		LatencyMS: latencyMS,
		Timestamp: time.Now().UnixMilli(),
	}
}
