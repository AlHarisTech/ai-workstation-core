package orchestratorv1

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type ChromaStore interface {
	StoreMemory(event *OrchestratorEvent) error
}

type ChromaConfig struct {
	BaseURL    string
	Collection string
}

type ChromaAdapter struct {
	config ChromaConfig
	client *http.Client
}

func NewChromaAdapter(config ChromaConfig) *ChromaAdapter {
	return &ChromaAdapter{
		config: config,
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (c *ChromaAdapter) StoreMemory(event *OrchestratorEvent) error {
	if c.config.BaseURL == "" {
		log.Printf("[orchestrator:chroma] no base URL configured, logging memory for trace %s", event.TraceID)
		c.logMemory(event)
		return nil
	}
	return c.store(event)
}

type chromaDoc struct {
	ID       string            `json:"id"`
	Metadata map[string]string `json:"metadata"`
	Document string            `json:"document"`
}

func (c *ChromaAdapter) store(event *OrchestratorEvent) error {
	resultJSON, _ := json.Marshal(event.Execution.Result)

	doc := chromaDoc{
		ID: event.TraceID,
		Metadata: map[string]string{
			"session_id": event.Context.SessionID,
			"tenant_id":  event.Context.TenantID,
			"server":     event.Execution.Server,
			"operation":  event.Execution.Operation,
		},
		Document: string(resultJSON),
	}

	collection := c.config.Collection
	if collection == "" {
		collection = "mcp_execution_memory"
	}

	body := map[string]any{
		"documents": []chromaDoc{doc},
	}
	payload, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/api/v1/collections/%s/add", c.config.BaseURL, collection)

	resp, err := c.client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("chroma api error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("chroma api returned %d", resp.StatusCode)
	}

	log.Printf("[orchestrator:chroma] stored memory for trace %s in collection %s", event.TraceID, collection)
	return nil
}

func (c *ChromaAdapter) logMemory(event *OrchestratorEvent) {
	resultJSON, _ := json.Marshal(event.Execution.Result)
	log.Printf("[orchestrator:chroma] memory trace=%s session=%s server=%s op=%s data=%s",
		event.TraceID, event.Context.SessionID, event.Execution.Server,
		event.Execution.Operation, string(resultJSON))
}
