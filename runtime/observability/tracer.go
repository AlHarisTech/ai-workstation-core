package observability

import (
	"encoding/json"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type Tracer struct {
	events []types.KernelEvent
}

func NewTracer() *Tracer {
	return &Tracer{
		events: make([]types.KernelEvent, 0, 64),
	}
}

func (t *Tracer) TraceEvent(event types.KernelEvent) {
	event.Timestamp = time.Now()
	t.events = append(t.events, event)
}

func (t *Tracer) TraceRequestIngress(requestID string) {
	t.TraceEvent(types.NewKernelEvent(types.EventRequestIngress, requestID, "", nil))
}

func (t *Tracer) TraceQueueEnqueue(requestID string) {
	t.TraceEvent(types.NewKernelEvent(types.EventQueueEnqueue, requestID, "", nil))
}

func (t *Tracer) TraceQueueReject(requestID string) {
	t.TraceEvent(types.NewKernelEvent(types.EventQueueReject, requestID, "", map[string]string{
		"reason": "BACKPRESSURE_LIMIT",
	}))
}

func (t *Tracer) TraceToolExecution(requestID, workerID string) {
	t.TraceEvent(types.NewKernelEvent(types.EventToolExecution, requestID, workerID, nil))
}

func (t *Tracer) TraceToolTimeout(requestID, workerID string) {
	t.TraceEvent(types.NewKernelEvent(types.EventToolTimeout, requestID, workerID, nil))
}

func (t *Tracer) TraceWorkerPanic(requestID, workerID string, panic interface{}) {
	payload, _ := json.Marshal(map[string]interface{}{"panic": panic})
	event := types.NewKernelEvent(types.EventWorkerPanic, requestID, workerID, nil)
	event.Payload = payload
	t.TraceEvent(event)
}

func (t *Tracer) Events() []types.KernelEvent {
	return t.events
}

func (t *Tracer) EventCount() int {
	return len(t.events)
}
