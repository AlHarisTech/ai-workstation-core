package types

import (
	"encoding/json"
	"fmt"
	"time"
)

type EventType string

const (
	EventRequestIngress  EventType = "REQUEST_INGRESS"
	EventQueueEnqueue    EventType = "QUEUE_ENQUEUE"
	EventQueueDequeue    EventType = "QUEUE_DEQUEUE"
	EventQueueReject     EventType = "QUEUE_REJECTED"
	EventPolicyEval      EventType = "POLICY_EVALUATION"
	EventPolicyDenial    EventType = "POLICY_DENIAL"
	EventToolExecution   EventType = "TOOL_EXECUTION"
	EventToolTimeout     EventType = "TOOL_TIMEOUT"
	EventToolError       EventType = "TOOL_ERROR"
	EventStateCommit     EventType = "STATE_COMMIT"
	EventResponseEmit    EventType = "RESPONSE_EMIT"
	EventWorkerPanic     EventType = "WORKER_PANIC"
	EventKernelStart     EventType = "KERNEL_STARTED"
	EventKernelStop      EventType = "KERNEL_STOPPED"
)

type KernelEvent struct {
	EventID   string          `json:"event_id"`
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	RequestID string          `json:"request_id"`
	WorkerID  string          `json:"worker_id"`
	Payload   json.RawMessage `json:"payload"`
	TraceID   string          `json:"trace_id"`
}

func NewKernelEvent(eventType EventType, requestID, workerID string, payload interface{}) KernelEvent {
	p, _ := json.Marshal(payload)
	return KernelEvent{
		EventID:   fmt.Sprintf("evt_%d", time.Now().UnixNano()),
		Type:      eventType,
		Timestamp: time.Now(),
		RequestID: requestID,
		WorkerID:  workerID,
		Payload:   p,
		TraceID:   requestID,
	}
}
