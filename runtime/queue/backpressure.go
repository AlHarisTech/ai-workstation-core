package queue

import (
	"encoding/json"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

func BuildRejectionEvent(requestID string) types.KernelEvent {
	payload, _ := json.Marshal(map[string]string{
		"reason": "BACKPRESSURE_LIMIT",
	})
	return types.KernelEvent{
		EventID:   "evt_reject_" + requestID,
		Type:      types.EventQueueReject,
		RequestID: requestID,
		TraceID:   requestID,
		Payload:   payload,
		Timestamp: types.KernelEvent{}.Timestamp,
	}
}
