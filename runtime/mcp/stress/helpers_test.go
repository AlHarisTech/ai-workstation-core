package stress

import (
	"encoding/json"
	"testing"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
)

func setupKernel(t *testing.T) *kernel.KernelEngine {
	t.Helper()
	cfg := kernel.DefaultConfig()
	cfg.WorkspaceRoot = "/home/asem/workspace"
	ke, err := kernel.NewKernelEngine(cfg, kernel.DefaultHardeningConfig())
	if err != nil {
		t.Fatalf("failed to create kernel: %v", err)
	}
	return ke
}

func mapToJSON(id, sessionID, projectID, tool, action string, payload map[string]any) json.RawMessage {
	m := map[string]any{
		"id":         id,
		"session_id": sessionID,
		"project_id": projectID,
		"tool":       tool,
		"action":     action,
		"payload":    payload,
	}
	data, _ := json.Marshal(m)
	return data
}


