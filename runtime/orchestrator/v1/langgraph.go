package orchestratorv1

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

type LangGraphRunner interface {
	Execute(event *OrchestratorEvent) (any, error)
}

type LangGraphConfig struct {
	ScriptPath string
	APIURL     string
}

type LangGraphAdapter struct {
	config LangGraphConfig
}

func NewLangGraphAdapter(config LangGraphConfig) *LangGraphAdapter {
	return &LangGraphAdapter{config: config}
}

func (l *LangGraphAdapter) Execute(event *OrchestratorEvent) (any, error) {
	if l.config.ScriptPath == "" && l.config.APIURL == "" {
		log.Printf("[orchestrator:langgraph] no script or api url configured, simulating for trace %s", event.TraceID)
		return l.simulate(event)
	}

	if l.config.APIURL != "" {
		return l.callAPI(event)
	}
	return l.callScript(event)
}

func (l *LangGraphAdapter) simulate(event *OrchestratorEvent) (any, error) {
	result := map[string]any{
		"trace_id":      event.TraceID,
		"workflow":      event.Execution.Server + "." + event.Execution.Operation,
		"steps":         2,
		"status":        "simulated",
		"message":       "LangGraph workflow completed (simulated — no script configured)",
	}
	log.Printf("[orchestrator:langgraph] simulated workflow for %s: %v", event.TraceID, result)
	return result, nil
}

func (l *LangGraphAdapter) callScript(event *OrchestratorEvent) (any, error) {
	input, _ := json.Marshal(event)
	cmd := exec.Command("python3", l.config.ScriptPath, string(input))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("langgraph script failed: %w", err)
	}
	var result any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("langgraph script output parse failed: %w", err)
	}
	return result, nil
}

func (l *LangGraphAdapter) callAPI(event *OrchestratorEvent) (any, error) {
	log.Printf("[orchestrator:langgraph] would POST to %s for trace %s", l.config.APIURL, event.TraceID)
	return map[string]any{
		"trace_id": event.TraceID,
		"status":   "accepted",
		"via":      "api",
	}, nil
}
