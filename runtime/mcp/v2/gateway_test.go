package mcpv2

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func setupTestRepo(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mcp-test-*")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "test").Run()

	initFile := filepath.Join(dir, "README.md")
	os.WriteFile(initFile, []byte("# test"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	return dir
}

func validRequest(workspace string) *MCPRequest {
	return &MCPRequest{
		ID:        "550e8400-e29b-41d4-a716-446655440000",
		Timestamp: "2026-06-10T20:00:00Z",
		Source:    "opencode",
		Type:      "mcp.request",
		Action: MCPAction{
			Type:      ActionGit,
			Operation: "status",
			Version:   "1.0",
		},
		Context: MCPContext{
			TenantID:  "t1",
			SessionID: "s1",
			TraceID:   "tr1",
			Workspace: struct {
				Path string "json:\"path\""
				Repo string "json:\"repo\""
			}{Path: workspace, Repo: "origin/main"},
		},
		Payload: struct {
			Resource   string         "json:\"resource\""
			Parameters map[string]any "json:\"parameters\""
		}{Resource: "repo", Parameters: map[string]any{}},
		Auth:   MCPAuth{Type: "bearer", Scope: []string{"read"}},
		Policy: MCPPolicy{Allow: []string{"git.*"}, Deny: []string{}},
		Meta: MCPMeta{
			Priority: "high",
			Timeout:  30000,
			Retry:    2,
			TraceID:  "trace-1",
			SpanID:   "span-1",
		},
	}
}

func TestGateway_ValidRequest(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()
	req := validRequest(workspace)
	resp := gw.Process(req)

	if resp.Status != "success" {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.Error.Message)
	}
	if resp.Execution.Server != "git" {
		t.Fatalf("expected git server, got %s", resp.Execution.Server)
	}
	if resp.Meta.TraceID != "trace-1" {
		t.Fatalf("expected trace-1, got %s", resp.Meta.TraceID)
	}
}

func TestGateway_InvalidAction(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Action.Type = "unknown"

	resp := gw.Process(req)
	if resp.Status != "error" {
		t.Fatal("expected error for unknown action")
	}
	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %s", resp.Error.Code)
	}
}

func TestGateway_PolicyDenied(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Policy.Deny = []string{"git.*"}

	resp := gw.Process(req)
	if resp.Status != "error" {
		t.Fatal("expected error for denied policy")
	}
	if resp.Error.Code != "POLICY_DENIED" {
		t.Fatalf("expected POLICY_DENIED, got %s", resp.Error.Code)
	}
}

func TestGateway_PolicyAllowlist(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Policy.Allow = []string{"filesystem.*"}

	resp := gw.Process(req)
	if resp.Status != "error" {
		t.Fatal("expected error for action not in allow list")
	}
	if resp.Error.Code != "POLICY_DENIED" {
		t.Fatalf("expected POLICY_DENIED, got %s", resp.Error.Code)
	}
}

func TestGateway_RouteNotFound(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Action.Operation = "nonexistent"

	resp := gw.Process(req)
	if resp.Status != "error" {
		t.Fatal("expected error for unsupported operation")
	}
	if resp.Error.Code != "ROUTE_NOT_FOUND" {
		t.Fatalf("expected ROUTE_NOT_FOUND, got %s", resp.Error.Code)
	}
}

func TestGateway_AllServers(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	tests := []struct {
		name      string
		action    ActionType
		operation string
		server    string
		payload   map[string]any
		wantFail  bool
	}{
		{"git-status", ActionGit, "status", "git", map[string]any{}, false},
		{"filesystem-read", ActionFilesystem, "read", "filesystem", map[string]any{"path": "README.md"}, false},
		{"memory-store", ActionMemory, "store", "memory", map[string]any{"key": "k1", "value": "v1"}, false},
		{"github-create-release", ActionGitHub, "create_release", "github", map[string]any{"owner": "test", "repo": "test", "tag": "v1"}, true},
		{"fetch-url", ActionFetch, "url", "fetch", map[string]any{"url": "https://example.com"}, true},
		{"context7-query", ActionContext7, "query", "context7", map[string]any{"key": "test"}, false},
		{"postgres-list-tables", ActionPostgres, "list_tables", "postgres", map[string]any{}, false},
		{"chroma-list-collections", ActionChromaDB, "list_collections", "chroma", map[string]any{}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(workspace)
			req.Action.Type = tc.action
			req.Action.Operation = tc.operation
			req.Policy = MCPPolicy{Allow: []string{string(tc.action) + ".*"}}
			req.Payload.Parameters = tc.payload

			resp := gw.Process(req)
			if tc.wantFail {
				if resp.Status == "success" {
					t.Logf("unexpected success: %v", resp.Result.Data)
				}
				return
			}
			if resp.Status != "success" {
				t.Fatalf("expected success, got %s: %s", resp.Status, resp.Error.Message)
			}
			if resp.Execution.Server != tc.server {
				t.Fatalf("expected server %s, got %s", tc.server, resp.Execution.Server)
			}
		})
	}
}

func TestGateway_JSONRoundTrip(t *testing.T) {
	workspace := setupTestRepo(t)
	req := validRequest(workspace)
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var parsed MCPRequest
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	gw := NewGateway()
	resp := gw.Process(&parsed)
	if resp.Status != "success" {
		t.Fatalf("expected success: %s", resp.Error.Message)
	}

	respRaw, _ := json.Marshal(resp)
	if !strings.Contains(string(respRaw), `"status":"success"`) {
		t.Fatal("response JSON missing success status")
	}
	t.Logf("Response: %s", string(respRaw))
}

func TestGateway_ValidationMissingFields(t *testing.T) {
	gw := NewGateway()
	req := &MCPRequest{}

	resp := gw.Process(req)
	if resp.Status != "error" {
		t.Fatal("expected error for empty request")
	}
}

func TestGateway_GenerateMissingTrace(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()
	req := validRequest(workspace)
	req.Meta.TraceID = ""
	req.Meta.SpanID = ""

	resp := gw.Process(req)
	if resp.Meta.TraceID == "" {
		t.Fatal("expected auto-generated trace_id")
	}
	if resp.Meta.SpanID == "" {
		t.Fatal("expected auto-generated span_id")
	}
}

func TestGateway_TimeoutPolicy(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Meta.Timeout = 999999

	resp := gw.Process(req)
	if resp.Status != "error" {
		t.Fatal("expected error for excessive timeout")
	}
	if resp.Error.Code != "VALIDATION_ERROR" {
		t.Fatalf("expected VALIDATION_ERROR, got %s", resp.Error.Code)
	}
}

func TestGateway_FilesystemWrite(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()
	req := validRequest(workspace)
	req.Action.Type = ActionFilesystem
	req.Action.Operation = "write"
	req.Policy = MCPPolicy{Allow: []string{"filesystem.*"}}
	req.Payload.Parameters = map[string]any{
		"path":    "newfile.txt",
		"content": "hello world",
	}

	resp := gw.Process(req)
	if resp.Status != "success" {
		t.Fatalf("expected success: %s", resp.Error.Message)
	}

	data, _ := json.Marshal(resp.Result.Data)
	if !strings.Contains(string(data), `"written":true`) {
		t.Fatal("expected write confirmation in response")
	}
}

func TestGateway_MemoryStoreRetrieve(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Action.Type = ActionMemory
	req.Action.Operation = "store"
	req.Policy = MCPPolicy{Allow: []string{"memory.*"}}
	req.Payload.Parameters = map[string]any{
		"key": "test-key",
		"value": "test-value",
	}

	resp := gw.Process(req)
	if resp.Status != "success" {
		t.Fatalf("store failed: %s", resp.Error.Message)
	}

	req2 := validRequest("/tmp")
	req2.Action.Type = ActionMemory
	req2.Action.Operation = "retrieve"
	req2.Policy = MCPPolicy{Allow: []string{"memory.*"}}
	req2.Payload.Parameters = map[string]any{"key": "test-key"}

	resp2 := gw.Process(req2)
	if resp2.Status != "success" {
		t.Fatalf("retrieve failed: %s", resp2.Error.Message)
	}
}

func TestGateway_GitCommit(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	os.WriteFile(filepath.Join(workspace, "new.go"), []byte("package main"), 0644)

	req := validRequest(workspace)
	req.Action.Operation = "commit"
	req.Payload.Parameters = map[string]any{"message": "test commit"}

	resp := gw.Process(req)
	if resp.Status != "success" {
		t.Fatalf("commit failed: %s", resp.Error.Message)
	}
	if resp.Execution.Server != "git" {
		t.Fatalf("expected git server, got %s", resp.Execution.Server)
	}
}
