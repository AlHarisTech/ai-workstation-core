package mcp

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
)

func tempWorkspace(t *testing.T) string {
	d, _ := os.MkdirTemp("", "mcp-int-*")
	os.MkdirAll(d+"/.ai/governance/audit", 0755)
	os.MkdirAll(d+"/.ai/state/traces", 0755)
	os.MkdirAll(d+"/.ai/state/sessions", 0755)
	os.MkdirAll(d+"/.ai/state/snapshots", 0755)
	t.Cleanup(func() { os.RemoveAll(d) })
	return d
}

func TestIntegration_FilesystemRead(t *testing.T) {
	ws := tempWorkspace(t)
	cfg := kernel.DefaultConfig()
	cfg.WorkspaceRoot = ws
	harden := kernel.DefaultHardeningConfig()
	ke, _ := kernel.NewKernelEngine(cfg, harden)
	ke.Start()

	ig := NewIntegrationGateway(ke)

	req := map[string]any{
		"id":         "fs_001",
		"session_id": "s1",
		"project_id": "p1",
		"tool":       "filesystem",
		"action":     "read",
		"payload":    map[string]any{"path": "/home/asem/workspace/VERSION"},
	}
	raw, _ := json.Marshal(req)

	resp := ig.Process(raw)
	if !resp.Success {
		t.Fatalf("filesystem read failed: %s", resp.Error)
	}
	data, ok := resp.Data.(map[string]any)
	if !ok {
		t.Fatalf("unexpected response type: %T", resp.Data)
	}
	if data["content"] == nil {
		t.Fatalf("no content returned")
	}
	t.Logf("[Filesystem Read] PASS: path=%s size=%v", data["path"], data["size"])
	ke.GracefulShutdown()
}

func TestIntegration_GitStatus(t *testing.T) {
	ws := tempWorkspace(t)
	cfg := kernel.DefaultConfig()
	cfg.WorkspaceRoot = ws
	harden := kernel.DefaultHardeningConfig()
	ke, _ := kernel.NewKernelEngine(cfg, harden)
	ke.Start()

	ig := NewIntegrationGateway(ke)

	req := map[string]any{
		"id":         "git_001",
		"session_id": "s1",
		"project_id": "p1",
		"tool":       "git",
		"action":     "status",
		"payload":    map[string]any{},
	}
	raw, _ := json.Marshal(req)

	resp := ig.Process(raw)
	if !resp.Success {
		t.Fatalf("git status failed: %s", resp.Error)
	}
	t.Logf("[Git Status] PASS: latency=%dms", resp.LatencyMS)

	// Test git log
	req2 := map[string]any{
		"id": "git_002", "session_id": "s1", "project_id": "p1",
		"tool": "git", "action": "log",
		"payload": map[string]any{"count": 3},
	}
	raw2, _ := json.Marshal(req2)

	resp2 := ig.Process(raw2)
	if !resp2.Success {
		t.Fatalf("git log failed: %s", resp2.Error)
	}
	t.Logf("[Git Log] PASS: latency=%dms", resp2.LatencyMS)
	ke.GracefulShutdown()
}

func TestIntegration_ToolList(t *testing.T) {
	ws := tempWorkspace(t)
	cfg := kernel.DefaultConfig()
	cfg.WorkspaceRoot = ws
	harden := kernel.DefaultHardeningConfig()
	ke, _ := kernel.NewKernelEngine(cfg, harden)
	ke.Start()

	ig := NewIntegrationGateway(ke)

	tools := ig.ListTools()
	t.Logf("[Tool List] %d tools registered", len(tools))
	for _, tool := range tools {
		actions, _ := tool["actions"].([]string)
		t.Logf("  %s: %v", tool["tool"], actions)
	}

	if len(tools) < 3 {
		t.Errorf("expected at least 3 tools, got %d", len(tools))
	}
	ke.GracefulShutdown()
}

func TestIntegration_SessionDenial(t *testing.T) {
	ws := tempWorkspace(t)
	cfg := kernel.DefaultConfig()
	cfg.WorkspaceRoot = ws
	harden := kernel.DefaultHardeningConfig()
	ke, _ := kernel.NewKernelEngine(cfg, harden)
	ke.Start()

	ig := NewIntegrationGateway(ke)

	req := map[string]any{
		"id":         "deny_001",
		"session_id": "",
		"project_id": "",
		"tool":       "filesystem",
		"action":     "read",
		"payload":    map[string]any{"path": "/etc/passwd"},
	}
	raw, _ := json.Marshal(req)

	resp := ig.Process(raw)
	time.Sleep(200 * time.Millisecond)

	if resp.Success {
		t.Error("session denial bypassed — should have failed")
	}
	t.Logf("[Session Denial] PASS: denied correctly")
	ke.GracefulShutdown()
}
