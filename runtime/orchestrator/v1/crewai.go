package orchestratorv1

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
)

type CrewAIRunner interface {
	Execute(event *OrchestratorEvent) (any, error)
}

type CrewAIConfig struct {
	ScriptPath string
	APIURL     string
}

type CrewAIAdapter struct {
	config CrewAIConfig
}

func NewCrewAIAdapter(config CrewAIConfig) *CrewAIAdapter {
	return &CrewAIAdapter{config: config}
}

func (c *CrewAIAdapter) Execute(event *OrchestratorEvent) (any, error) {
	if c.config.ScriptPath == "" && c.config.APIURL == "" {
		log.Printf("[orchestrator:crewai] no script or api url configured, simulating for trace %s", event.TraceID)
		return c.simulate(event)
	}
	if c.config.APIURL != "" {
		return c.callAPI(event)
	}
	return c.callScript(event)
}

func (c *CrewAIAdapter) simulate(event *OrchestratorEvent) (any, error) {
	agents := []string{"researcher", "writer", "reviewer"}
	if event.Execution.Server == "context7" {
		agents = []string{"context_loader", "resolver"}
	}

	result := map[string]any{
		"trace_id": event.TraceID,
		"task":     event.Execution.Server + "." + event.Execution.Operation,
		"agents":   agents,
		"status":   "simulated",
		"message":  "CrewAI execution completed (simulated — no script configured)",
	}
	log.Printf("[orchestrator:crewai] simulated crew for %s: agents=%v", event.TraceID, agents)
	return result, nil
}

func (c *CrewAIAdapter) callScript(event *OrchestratorEvent) (any, error) {
	input, _ := json.Marshal(event)
	cmd := exec.Command("python3", c.config.ScriptPath, string(input))
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("crewai script failed: %w", err)
	}
	var result any
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, fmt.Errorf("crewai script output parse failed: %w", err)
	}
	return result, nil
}

func (c *CrewAIAdapter) callAPI(event *OrchestratorEvent) (any, error) {
	log.Printf("[orchestrator:crewai] would POST to %s for trace %s", c.config.APIURL, event.TraceID)
	return map[string]any{
		"trace_id": event.TraceID,
		"status":   "dispatched",
		"via":      "api",
	}, nil
}
