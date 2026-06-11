package orchestratorv1

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func mockEvent(t *testing.T) *OrchestratorEvent {
	t.Helper()
	return &OrchestratorEvent{
		TraceID:   "trace-abc-123",
		Source:    "mcp-gateway",
		EventType: "execution.completed",
		Execution: EventExecution{
			Server:    "git",
			Operation: "status",
			Result:    map[string]any{"branch": "main", "clean": true},
		},
		Context: EventContext{
			SessionID: "session-1",
			TenantID:  "tenant-1",
		},
	}
}

func TestOrchestrator_ProcessBasic(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	resp := o.Process(event)

	if resp.Status != "completed" {
		t.Fatalf("expected completed, got %s", resp.Status)
	}
	if resp.TraceID != "trace-abc-123" {
		t.Fatalf("expected trace-abc-123, got %s", resp.TraceID)
	}
	if resp.Result == nil {
		t.Fatal("expected result to be passed through")
	}
}

func TestOrchestrator_SystemsTriggered(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	resp := o.Process(event)

	found := map[string]bool{}
	for _, s := range resp.SystemsTriggered {
		found[s] = true
	}
	if !found["postgres"] {
		t.Fatal("expected postgres in triggered systems")
	}
	if !found["chroma"] {
		t.Fatal("expected chroma in triggered systems")
	}
}

func TestOrchestrator_LangGraphTriggeredForCommit(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	event.Execution.Operation = "commit"

	resp := o.Process(event)
	found := false
	for _, s := range resp.SystemsTriggered {
		if s == "langgraph" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected langgraph triggered for commit operation")
	}
}

func TestOrchestrator_CrewAITriggeredForResolve(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	event.Execution.Operation = "resolve"

	resp := o.Process(event)
	found := false
	for _, s := range resp.SystemsTriggered {
		if s == "crewai" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected crewai triggered for resolve operation")
	}
}

func TestOrchestrator_NonBlockingOnPanic(t *testing.T) {
	o := NewOrchestrator(
		WithPostgres(&panicStore{}),
		WithChroma(&panicStore{}),
	)
	event := mockEvent(t)
	resp := o.Process(event)

	if resp.Status != "completed" {
		t.Fatalf("expected completed despite panics, got %s", resp.Status)
	}
}

type panicStore struct{}

func (p *panicStore) StoreExecution(event *OrchestratorEvent) error {
	panic("postgres panic")
}

func (p *panicStore) StoreMemory(event *OrchestratorEvent) error {
	panic("chroma panic")
}

func TestOrchestrator_AllSystemsTriggered(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	event.Execution.Operation = "commit"

	resp := o.Process(event)
	if len(resp.SystemsTriggered) == 0 {
		t.Fatal("expected at least one system triggered")
	}
}

func TestOrchestrator_ResultPassthrough(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	event.Execution.Result = map[string]any{"key": "value", "num": 42}

	resp := o.Process(event)
	data, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(data), `"key":"value"`) {
		t.Fatalf("expected result passthrough, got %s", data)
	}
}

func TestOrchestrator_MultipleEventsSameSession(t *testing.T) {
	o := NewOrchestrator()
	event1 := mockEvent(t)
	event1.Execution.Operation = "status"
	event2 := mockEvent(t)
	event2.TraceID = "trace-xyz-456"
	event2.Execution.Operation = "commit"

	r1 := o.Process(event1)
	r2 := o.Process(event2)

	if r1.TraceID != "trace-abc-123" {
		t.Fatalf("expected trace-abc-123, got %s", r1.TraceID)
	}
	if r2.TraceID != "trace-xyz-456" {
		t.Fatalf("expected trace-xyz-456, got %s", r2.TraceID)
	}
}

func TestOrchestrator_ConcurrentEvents(t *testing.T) {
	o := NewOrchestrator()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			event := mockEvent(t)
			event.TraceID = "trace-" + string(rune('0'+i))
			resp := o.Process(event)
			if resp.Status != "completed" {
				t.Errorf("expected completed for event %d", i)
			}
		}(i)
	}
	wg.Wait()
}

func TestOrchestrator_LangGraphSimulated(t *testing.T) {
	lg := NewLangGraphAdapter(LangGraphConfig{})
	event := mockEvent(t)
	result, err := lg.Execute(event)
	if err != nil {
		t.Fatalf("langgraph simulate failed: %v", err)
	}
	data, _ := json.Marshal(result)
	if !strings.Contains(string(data), "simulated") {
		t.Fatalf("expected simulated result, got %s", data)
	}
}

func TestOrchestrator_CrewAISimulated(t *testing.T) {
	ca := NewCrewAIAdapter(CrewAIConfig{})
	event := mockEvent(t)
	result, err := ca.Execute(event)
	if err != nil {
		t.Fatalf("crewai simulate failed: %v", err)
	}
	data, _ := json.Marshal(result)
	if !strings.Contains(string(data), "simulated") {
		t.Fatalf("expected simulated result, got %s", data)
	}
}

func TestOrchestrator_PostgresFallback(t *testing.T) {
	pg := NewPostgresAdapter(PostgresConfig{})
	err := pg.StoreExecution(mockEvent(t))
	if err != nil {
		t.Fatalf("postgres fallback should not error: %v", err)
	}
}

func TestOrchestrator_ChromaFallback(t *testing.T) {
	ch := NewChromaAdapter(ChromaConfig{})
	err := ch.StoreMemory(mockEvent(t))
	if err != nil {
		t.Fatalf("chroma fallback should not error: %v", err)
	}
}

func TestOrchestrator_CustomDecider(t *testing.T) {
	custom := &customDecider{}
	o := NewOrchestrator(WithDecider(custom))
	event := mockEvent(t)
	resp := o.Process(event)

	if len(resp.SystemsTriggered) > 0 {
		t.Fatal("expected no systems triggered with custom decider that rejects all")
	}
}

type customDecider struct{}

func (c *customDecider) Decide(event *OrchestratorEvent) RoutingDecision {
	return RoutingDecision{}
}

func TestOrchestrator_InputContractJSON(t *testing.T) {
	raw := `{
		"trace_id": "uuid-1",
		"source": "mcp-gateway",
		"event_type": "execution.completed",
		"execution": {
			"server": "git",
			"operation": "status",
			"result": {"branch": "main"}
		},
		"context": {
			"session_id": "s1",
			"tenant_id": "t1"
		}
	}`

	var event OrchestratorEvent
	if err := json.Unmarshal([]byte(raw), &event); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	o := NewOrchestrator()
	resp := o.Process(&event)

	if resp.TraceID != "uuid-1" {
		t.Fatalf("expected uuid-1, got %s", resp.TraceID)
	}
}

func TestOrchestrator_OutputContractJSON(t *testing.T) {
	o := NewOrchestrator()
	event := mockEvent(t)
	resp := o.Process(event)

	out, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]any
	json.Unmarshal(out, &decoded)

	if decoded["trace_id"] != "trace-abc-123" {
		t.Fatal("missing trace_id in output")
	}
	if decoded["status"] != "completed" {
		t.Fatal("missing status in output")
	}
	if _, ok := decoded["systems_triggered"]; !ok {
		t.Fatal("missing systems_triggered in output")
	}
	if _, ok := decoded["result"]; !ok {
		t.Fatal("missing result in output")
	}
}
