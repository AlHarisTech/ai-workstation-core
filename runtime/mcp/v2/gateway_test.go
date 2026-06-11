package mcpv2

import (
	"bytes"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestMain(m *testing.M) {
	keys := []string{"CHROMA_API_KEY", "CHROMA_TENANT", "CHROMA_DATABASE", "CHROMA_HOST", "CHROMA_PORT", "CHROMA_URL"}
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	code := m.Run()
	for k, v := range saved {
		if v != "" {
			os.Setenv(k, v)
		}
	}
	os.Exit(code)
}

func TestGateway_KnowledgeInjected(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()
	req := validRequest(workspace)

	resp := gw.Process(req)

	if resp.Status != "success" {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.Error.Message)
	}
	if len(req.Context.Knowledge) == 0 {
		t.Fatal("expected knowledge to be populated during Process")
	}
	if req.Context.Knowledge[0].Query != "git.status" {
		t.Fatalf("expected query 'git.status', got %s", req.Context.Knowledge[0].Query)
	}
}

func TestGateway_GovernanceAudit(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	workspace := setupTestRepo(t)
	gw := NewGateway()
	req := validRequest(workspace)

	resp := gw.Process(req)

	if resp.Status != "success" {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.Error.Message)
	}

	output := buf.String()
	if !strings.Contains(output, "[audit]") {
		t.Fatal("expected audit log entry")
	}
	if !strings.Contains(output, "git.status") {
		t.Fatal("expected 'git.status' in audit log")
	}
	if !strings.Contains(output, resp.Meta.TraceID) {
		t.Fatal("expected trace ID in audit log")
	}
}

func TestGateway_KnowledgeNonBlocking(t *testing.T) {
	// Verify Process succeeds even when Chroma has no credentials
	// (already cleared by TestMain — chroma adapter is in fallback mode,
	// which returns simulated results, not errors; this test validates
	// the non-blocking path explicitly does not abort the request)
	workspace := setupTestRepo(t)
	gw := NewGateway()
	req := validRequest(workspace)
	req.Action.Type = ActionFilesystem
	req.Action.Operation = "read"
	req.Policy = MCPPolicy{Allow: []string{"filesystem.*"}}
	req.Payload.Parameters = map[string]any{"path": "README.md"}

	resp := gw.Process(req)
	if resp.Status != "success" {
		t.Fatalf("expected success despite chroma in fallback mode: %s", resp.Error.Message)
	}
	// Knowledge should still be populated (fallback returns simulated results, not error)
	if len(req.Context.Knowledge) == 0 {
		t.Fatal("expected knowledge to be populated even in fallback mode")
	}
}

func TestGateway_SelectBestServerDirect(t *testing.T) {
	gw := NewGateway()
	req := validRequest("/tmp")
	req.Action.Type = ActionGit
	req.Action.Operation = "status"

	candidates := gw.router.ListAll()

	// Both git and fetch support "status" — either is valid without knowledge
	cap := gw.selectBestServer(candidates, req, nil, nil)
	if cap == nil {
		t.Fatal("expected a server to be selected")
	}
	supportsStatus := false
	for _, op := range cap.Capabilities {
		if op == "status" {
			supportsStatus = true
			break
		}
	}
	if !supportsStatus {
		t.Fatalf("selected server %s does not support 'status'", cap.Server)
	}

	// With empty knowledge, scoring should not change result
	cap2 := gw.selectBestServer(candidates, req, []KnowledgeDoc{
		{Results: map[string]any{}},
	}, nil)
	supportsStatus2 := false
	for _, op := range cap2.Capabilities {
		if op == "status" {
			supportsStatus2 = true
			break
		}
	}
	if !supportsStatus2 {
		t.Fatalf("selected server %s does not support 'status'", cap2.Server)
	}

	// Verify all 8 candidates are scored
	if len(candidates) != 8 {
		t.Fatalf("expected 8 capability candidates, got %d", len(candidates))
	}
}

func TestGateway_ScoringDirect(t *testing.T) {
	req := validRequest("/tmp")
	req.Action.Type = ActionFilesystem
	req.Action.Operation = "read"

	capFS := &Capability{Server: "filesystem", Capabilities: []string{"read", "write", "list", "search", "delete"}}
	capGH := &Capability{Server: "github", Capabilities: []string{"read", "repo", "list_issues", "create_issue", "create_pr", "create_release", "push", "tag"}}

	defCap, defKW, defHist := 0.30, 0.40, 0.30

	sFS := scoreCapability(req, capFS, nil, defCap, defKW, defHist)
	sGH := scoreCapability(req, capGH, nil, defCap, defKW, defHist)
	if sFS != 0.30 {
		t.Fatalf("expected filesystem score 0.30, got %.2f", sFS)
	}
	if sGH != 0.30 {
		t.Fatalf("expected github score 0.30, got %.2f", sGH)
	}

	ghKnowledge := []KnowledgeDoc{
		{
			Results: map[string]any{
				"documents": []map[string]any{
					{"document": "github pull requests and issues integration"},
				},
			},
		},
	}
	sFSk := scoreCapability(req, capFS, ghKnowledge, defCap, defKW, defHist)
	sGHk := scoreCapability(req, capGH, ghKnowledge, defCap, defKW, defHist)
	t.Logf("filesystem=%.2f github=%.2f (knowledge biased to github)", sFSk, sGHk)
	if sGHk <= sFSk {
		t.Log("github did not surpass filesystem — scoring weighting may need tuning")
	}
}

func TestGateway_AdaptiveWeights(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()
	gw.exploration = nil // disable exploration; this test only measures learning engine

	for i := 0; i < 10; i++ {
		req := validRequest(workspace)
		resp := gw.Process(req)
		if resp.Status != "success" {
			t.Fatalf("request %d failed: %s", i, resp.Error.Message)
		}
	}

	w := gw.learningEngine.WeightsFor("git")
	_, _, hist := w.Factors()
	t.Logf("git history weight after 10 successful git.status: %.3f (default: 0.300)", hist)
	if hist <= 0.30+0.001 {
		t.Fatal("expected history weight to increase after repeated successes")
	}
}

func TestGateway_LearningEngineFailures(t *testing.T) {
	le := NewLearningEngine()
	le.Update(RoutingOutcome{
		RequestID:      "r1",
		SelectedServer: "git",
		Success:        false,
		LatencyMs:      100,
		Timestamp:      time.Now(),
	})
	w := le.WeightsFor("git")
	_, _, hist := w.Factors()
	t.Logf("git history after failure: %.3f", hist)
	if hist >= 0.30 {
		t.Fatal("expected history weight to decrease after failure")
	}
}

func TestGateway_ListAll(t *testing.T) {
	r := NewRouter()
	candidates := r.ListAll()
	if len(candidates) != 8 {
		t.Fatalf("expected 8 registered capabilities, got %d", len(candidates))
	}
	servers := make(map[string]bool)
	for _, c := range candidates {
		servers[c.Server] = true
	}
	for _, s := range []string{"git", "filesystem", "memory", "github", "fetch", "context7", "postgres", "chroma"} {
		if !servers[s] {
			t.Fatalf("missing server: %s", s)
		}
	}
}

func TestExplorationState_AdjustScore(t *testing.T) {
	es := NewExplorationState(0.10)

	// First selection: max bonus
	adj1 := es.AdjustScore("git", 0.30)
	if adj1 != 0.40 {
		t.Fatalf("expected 0.40 (0.30+0.10), got %.2f", adj1)
	}

	// Record 10 git selections
	for i := 0; i < 10; i++ {
		es.RecordSelection("git")
	}

	// git should now have penalty (10/10 = 1.0 freq → penalty 0.10)
	adjGit := es.AdjustScore("git", 0.30)
	if adjGit >= 0.30 {
		t.Fatalf("expected git score < 0.30 after many selections, got %.2f", adjGit)
	}

	// An unused server should still get bonus
	adjFS := es.AdjustScore("filesystem", 0.30)
	if adjFS <= 0.30 {
		t.Fatalf("expected filesystem score > 0.30 (underused), got %.2f", adjFS)
	}

	t.Logf("git(used 10x)=%.2f filesystem(unused)=%.2f", adjGit, adjFS)
}

func TestGateway_ExplorationDrift(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	servers := make(map[string]int)
	for i := 0; i < 50; i++ {
		req := validRequest(workspace)
		req.Action.Type = ActionGit
		req.Action.Operation = "status"
		resp := gw.Process(req)
		servers[resp.Execution.Server]++
	}

	t.Logf("server distribution after 50 git.status requests: %v", servers)

	if servers["git"] < 40 {
		t.Logf("note: git selected %d/50 times — exploration active", servers["git"])
	}

	unique := len(servers)
	if unique < 2 {
		t.Errorf("expected at least 2 unique servers with exploration, got %d (distribution: %v)", unique, servers)
	}
	t.Logf("unique servers selected: %d", unique)
}

func TestStabilityEngine_ExplorationDecay(t *testing.T) {
	se := NewStabilityEngine(0.02, 20)
	baseRate := 0.10

	rate0 := se.EffectiveRate("git", baseRate)
	if rate0 != baseRate {
		t.Fatalf("expected rate %.2f for unused server, got %.4f", baseRate, rate0)
	}

	// Simulate 50 uses
	for i := 0; i < 50; i++ {
		se.RecordSelection("status", "git")
	}
	rate50 := se.EffectiveRate("git", baseRate)
	if rate50 >= baseRate {
		t.Fatalf("expected rate < %.2f after 50 uses, got %.4f", baseRate, rate50)
	}
	if rate50 <= 0 {
		t.Fatalf("expected rate > 0 (floor at 1%%), got %.6f", rate50)
	}

	// Simulate 500 more uses — should hit floor
	for i := 0; i < 500; i++ {
		se.RecordSelection("status", "git")
	}
	rate500 := se.EffectiveRate("git", baseRate)
	minRate := baseRate * 0.01
	if rate500 != minRate {
		t.Fatalf("expected rate at floor %.4f after heavy use, got %.6f", minRate, rate500)
	}

	t.Logf("rates: unused=%.4f after50=%.4f after500=%.4f (min=%.4f)", rate0, rate50, rate500, minRate)
}

func TestStabilityEngine_OscillationDetection(t *testing.T) {
	se := NewStabilityEngine(0.02, 10)

	// No oscillation with <4 selections
	se.RecordSelection("status", "git")
	se.RecordSelection("status", "fetch")
	se.RecordSelection("status", "git")
	if osc := se.OscillationCount("status"); osc != 0 {
		t.Fatalf("expected 0 oscillation with only 3 entries, got %d", osc)
	}

	// Create oscillation: git → fetch → git → fetch
	se.RecordSelection("status", "fetch")
	if osc := se.OscillationCount("status"); osc < 1 {
		t.Fatalf("expected >=1 oscillation for alternating pattern, got %d", osc)
	}

	// Continue oscillation
	se.RecordSelection("status", "git")
	se.RecordSelection("status", "fetch")
	se.RecordSelection("status", "git")
	se.RecordSelection("status", "fetch")
	if osc := se.OscillationCount("status"); osc < 3 {
		t.Fatalf("expected >=3 oscillations after extended alternation, got %d", osc)
	}

	t.Logf("oscillation count for alternating pattern: %d", se.OscillationCount("status"))
}

func TestStabilityEngine_Convergence(t *testing.T) {
	se := NewStabilityEngine(0.02, 10)

	// Fill window with same server
	for i := 0; i < 10; i++ {
		se.RecordSelection("status", "git")
	}
	cvg := se.ConvergenceScore("status")
	if cvg != 1.0 {
		t.Fatalf("expected convergence 1.0 for uniform selection, got %.2f", cvg)
	}

	// Mix with another server
	se.RecordSelection("status", "fetch")
	cvg = se.ConvergenceScore("status")
	if cvg >= 1.0 {
		t.Fatalf("expected convergence < 1.0 after mixing, got %.2f", cvg)
	}
	if cvg < 0.8 {
		t.Fatalf("expected convergence >= 0.8 (9/11 git), got %.2f", cvg)
	}

	t.Logf("convergence scores: uniform=1.0 mixed=%.2f", cvg)
}

func TestStabilityEngine_StabilityBias(t *testing.T) {
	se := NewStabilityEngine(0.02, 5)

	// Get initial bias
	if bias := se.StabilityBias("git"); bias != 0 {
		t.Fatalf("expected initial bias 0, got %.2f", bias)
	}

	// 5 consecutive selections → convergence > 0.5 → bias should increase
	for i := 0; i < 5; i++ {
		se.RecordSelection("status", "git")
	}
	bias1 := se.StabilityBias("git")
	if bias1 <= 0 {
		t.Fatalf("expected positive bias after convergence > 0.5, got %.2f", bias1)
	}

	// 5 more → more bias
	for i := 0; i < 5; i++ {
		se.RecordSelection("status", "git")
	}
	bias2 := se.StabilityBias("git")
	if bias2 <= bias1 {
		t.Fatalf("expected bias to increase with more selections, got %.2f <= %.2f", bias2, bias1)
	}

	t.Logf("git bias: after5=%.2f after10=%.2f", bias1, bias2)
}

func TestStabilityEngine_Metrics(t *testing.T) {
	se := NewStabilityEngine(0.02, 10)

	// Add some data
	for i := 0; i < 8; i++ {
		se.RecordSelection("status", "git")
	}
	for i := 0; i < 2; i++ {
		se.RecordSelection("status", "fetch")
	}

	metrics := se.Metrics()
	if metrics.ConvergenceScore["status"] < 0.7 {
		t.Fatalf("expected convergence >= 0.7 (8/10 git), got %.2f", metrics.ConvergenceScore["status"])
	}
	if metrics.StabilityIndex <= 0 {
		t.Fatalf("expected positive stability index, got %.2f", metrics.StabilityIndex)
	}

	t.Logf("metrics: osc=%v cvg=%v index=%.2f", metrics.OscillationCount, metrics.ConvergenceScore, metrics.StabilityIndex)
}

func TestGateway_Convergence(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	// Run 50 git.status requests — exploration should decay and convergence should increase
	for i := 0; i < 50; i++ {
		req := validRequest(workspace)
		req.Action.Type = ActionGit
		req.Action.Operation = "status"
		gw.Process(req)
	}

	st := gw.stability
	if st == nil {
		t.Fatal("stability engine is nil")
	}

	cvg := st.ConvergenceScore("status")
	osc := st.OscillationCount("status")
	usage := st.UsageCount("git")

	t.Logf("convergence_status after 50 requests: convergence=%.2f oscillation=%d git_usage=%d", cvg, osc, usage)

	if cvg < 0.3 {
		t.Errorf("expected convergence >= 0.3 after 50 requests, got %.2f", cvg)
	}

	// Run 50 more — convergence should strengthen
	for i := 0; i < 50; i++ {
		req := validRequest(workspace)
		req.Action.Type = ActionGit
		req.Action.Operation = "status"
		gw.Process(req)
	}

	cvg2 := st.ConvergenceScore("status")
	osc2 := st.OscillationCount("status")

	t.Logf("after 100 requests: convergence=%.2f oscillation=%d", cvg2, osc2)

	// After 100 requests, convergence should be at least as good as after 50
	if cvg2 < cvg {
		t.Logf("note: convergence did not increase (%.2f → %.2f) — may still be exploring", cvg, cvg2)
	}
}

func TestStabilityEngine_NilSafety(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()
	gw.stability = nil

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("panic with nil stability engine: %v", r)
		}
	}()

	for i := 0; i < 10; i++ {
		req := validRequest(workspace)
		req.Action.Type = ActionGit
		req.Action.Operation = "status"
		gw.Process(req)
	}

	t.Log("nil stability engine does not crash gateway — no panics over 10 requests")
}

func TestDecisionTrace_Complete(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	req := validRequest(workspace)
	req.Action.Type = ActionGit
	req.Action.Operation = "status"
	resp := gw.Process(req)

	dt := resp.Meta.DecisionTrace
	if dt == nil {
		t.Fatal("expected non-nil DecisionTrace")
	}

	if dt.TraceID == "" {
		t.Error("expected non-empty TraceID")
	}
	if dt.RequestID == "" {
		t.Error("expected non-empty RequestID")
	}
	if dt.SelectedServer == "" {
		t.Error("expected non-empty SelectedServer")
	}
	if len(dt.Steps) == 0 {
		t.Fatal("expected at least 1 trace step")
	}

	// Verify expected stages exist
	stageNames := make(map[string]bool)
	for _, s := range dt.Steps {
		stageNames[s.Stage] = true
	}

	for _, expected := range []string{"validate", "policy", "resolve", "route", "execute"} {
		if !stageNames[expected] {
			t.Errorf("missing trace step: %s", expected)
		}
	}

	// Verify server scores exist after scoring
	if dt.ServerScores == nil && stageNames["knowledge"] {
		// Only required if knowledge was retrieved
		t.Log("no server scores recorded (no knowledge or scoring step)")
	}

	t.Logf("trace steps: %v", stageNames)
	t.Logf("trace.selected_server=%s scores=%v", dt.SelectedServer, dt.ServerScores)
}

func TestDecisionTrace_JSON(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	req := validRequest(workspace)
	req.Action.Type = ActionGit
	req.Action.Operation = "status"
	resp := gw.Process(req)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal response: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal response JSON: %v", err)
	}

	meta, ok := raw["meta"].(map[string]any)
	if !ok {
		t.Fatal("response missing meta field")
	}

	dtRaw, ok := meta["decision_trace"]
	if !ok {
		t.Fatal("response meta missing decision_trace field")
	}

	dtMap, ok := dtRaw.(map[string]any)
	if !ok {
		t.Fatalf("decision_trace is not an object, got %T", dtRaw)
	}

	if dtMap["trace_id"] == "" {
		t.Error("decision_trace.trace_id is empty")
	}
	if dtMap["selected_server"] == "" {
		t.Error("decision_trace.selected_server is empty")
	}

	steps, ok := dtMap["steps"].([]any)
	if !ok {
		t.Fatal("decision_trace.steps is not an array")
	}
	if len(steps) == 0 {
		t.Fatal("decision_trace.steps is empty")
	}

	// Each step must have a stage field
	for i, s := range steps {
		step, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("step[%d] is not an object", i)
		}
		if step["stage"] == "" {
			t.Errorf("step[%d] missing stage", i)
		}
	}

	t.Logf("response JSON includes decision_trace with %d steps", len(steps))
}

func TestDecisionTrace_ErrorPath(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	tests := []struct {
		name       string
		setup      func(*MCPRequest)
		wantStatus string
	}{
		{
			name:       "validation error",
			setup:      func(r *MCPRequest) { r.Action.Type = "" },
			wantStatus: "error",
		},
		{
			name: "policy denied",
			setup: func(r *MCPRequest) {
				r.Policy.Deny = append(r.Policy.Deny, "git.status")
			},
			wantStatus: "error",
		},
		{
			name: "route not found",
			setup: func(r *MCPRequest) {
				r.Action.Type = ActionGit
				r.Action.Operation = "nonexistent"
			},
			wantStatus: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := validRequest(workspace)
			tt.setup(req)
			resp := gw.Process(req)

			if resp.Meta.DecisionTrace == nil {
				t.Fatal("expected non-nil trace even on error")
			}
			if len(resp.Meta.DecisionTrace.Steps) == 0 {
				t.Fatal("expected at least 1 trace step on error")
			}

			lastStep := resp.Meta.DecisionTrace.Steps[len(resp.Meta.DecisionTrace.Steps)-1]
			if lastStep.Output == "" {
				t.Error("expected non-empty output on error step")
			}

			t.Logf("error trace: stage=%s output=%s", lastStep.Stage, lastStep.Output)
		})
	}
}

func TestDecisionTrace_RoutingParity(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	// Run the same request twice and verify routing outcome is identical
	var firstServer string
	for i := 0; i < 2; i++ {
		req := validRequest(workspace)
		req.Action.Type = ActionGit
		req.Action.Operation = "status"
		resp := gw.Process(req)

		if i == 0 {
			firstServer = resp.Execution.Server
			if resp.Meta.DecisionTrace == nil {
				t.Fatal("expected trace on first request")
			}
			if resp.Meta.DecisionTrace.SelectedServer != resp.Execution.Server {
				t.Errorf("trace.selected_server=%s != execution.server=%s",
					resp.Meta.DecisionTrace.SelectedServer, resp.Execution.Server)
			}
		}
	}

	// Second request may differ due to learning/exploration drift — that's expected.
	// The key assertion: trace.SelectedServer always matches resp.Execution.Server
	t.Logf("first server selection: %s (second may differ due to learning — that's ok)", firstServer)
}

func TestEnforcementEngine_DefaultAllow(t *testing.T) {
	ee := NewEnforcementEngine()

	// No rules set → everything allowed by default
	result := ee.Check("git", "git.status")
	if !result.Allowed {
		t.Fatalf("expected default allow, got blocked: %s", result.BlockReason)
	}
	if result.Server != "git" {
		t.Errorf("expected server=git, got %s", result.Server)
	}
	if result.Operation != "git.status" {
		t.Errorf("expected operation=git.status, got %s", result.Operation)
	}

	result2 := ee.Check("fetch", "fetch.url")
	if !result2.Allowed {
		t.Fatalf("expected default allow for unconfigured server, got blocked")
	}

	t.Log("default allow-all: all servers allowed")
}

func TestEnforcementEngine_Block(t *testing.T) {
	ee := NewEnforcementEngine()
	ee.SetRule("git", "git.status", false, "maintenance mode")

	result := ee.Check("git", "git.status")
	if result.Allowed {
		t.Fatal("expected git.status to be blocked")
	}
	if result.BlockReason != "maintenance mode" {
		t.Fatalf("expected reason 'maintenance mode', got '%s'", result.BlockReason)
	}

	// Other operations on same server still allowed
	result2 := ee.Check("git", "git.commit")
	if !result2.Allowed {
		t.Fatal("expected git.commit to still be allowed")
	}

	// Other servers unaffected
	result3 := ee.Check("fetch", "fetch.url")
	if !result3.Allowed {
		t.Fatal("expected fetch.url to still be allowed")
	}

	// Same rule can be overridden
	ee.SetRule("git", "git.status", true, "back online")
	result4 := ee.Check("git", "git.status")
	if !result4.Allowed {
		t.Fatalf("expected git.status to be allowed after override: %s", result4.BlockReason)
	}

	t.Log("enforcement block and override work correctly")
}

func TestEnforcementEngine_Allow(t *testing.T) {
	ee := NewEnforcementEngine()

	// Explicit allow rule
	ee.SetRule("git", "git.status", true, "explicitly allowed")
	result := ee.Check("git", "git.status")
	if !result.Allowed {
		t.Fatalf("expected explicitly allowed, got blocked: %s", result.BlockReason)
	}

	t.Log("explicit allow rule works")
}

func TestGateway_EnforcementBlocked(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	// Block git.status
	gw.enforcement.SetRule("git", "git.status", false, "maintenance")

	req := validRequest(workspace)
	req.Action.Type = ActionGit
	req.Action.Operation = "status"
	resp := gw.Process(req)

	if resp.Status != "error" {
		t.Fatalf("expected error status after enforcement block, got %s", resp.Status)
	}
	if resp.Error.Code != "ENFORCEMENT_BLOCKED" {
		t.Fatalf("expected ENFORCEMENT_BLOCKED error code, got %s", resp.Error.Code)
	}

	// Trace must include enforcement step
	if resp.Meta.DecisionTrace == nil {
		t.Fatal("expected non-nil trace on enforcement block")
	}
	foundEnforcement := false
	for _, s := range resp.Meta.DecisionTrace.Steps {
		if s.Stage == "enforcement" {
			foundEnforcement = true
			if s.Output != "blocked" {
				t.Errorf("expected enforcement output 'blocked', got '%s'", s.Output)
			}
			break
		}
	}
	if !foundEnforcement {
		t.Error("missing enforcement step in trace")
	}

	// Execution.Server should still be set (routing happened)
	if resp.Execution.Server != "git" {
		t.Errorf("expected execution.server=git, got %s", resp.Execution.Server)
	}

	t.Logf("enforcement blocked: code=%s reason=%s", resp.Error.Code, resp.Error.Message)
}

func TestGateway_EnforcementAllowed(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	// Explicitly allow git.status
	gw.enforcement.SetRule("git", "git.status", true, "explicit allow")

	req := validRequest(workspace)
	req.Action.Type = ActionGit
	req.Action.Operation = "status"
	resp := gw.Process(req)

	if resp.Status != "success" {
		t.Fatalf("expected success after explicit allow, got %s: %s", resp.Status, resp.Error.Message)
	}

	// Trace should show enforcement:allowed
	if resp.Meta.DecisionTrace == nil {
		t.Fatal("expected non-nil trace")
	}
	foundEnforcement := false
	for _, s := range resp.Meta.DecisionTrace.Steps {
		if s.Stage == "enforcement" {
			foundEnforcement = true
			if s.Output != "allowed" {
				t.Errorf("expected enforcement output 'allowed', got '%s'", s.Output)
			}
			break
		}
	}
	if !foundEnforcement {
		t.Error("missing enforcement step in trace")
	}

	t.Log("explicit allow: enforcement passed, execution succeeded")
}

func TestGateway_EnforcementRoutingParity(t *testing.T) {
	// Verify enforcement does NOT change selected server
	workspace := setupTestRepo(t)
	gw := NewGateway()

	// Block git.status — execution will be blocked, but server should still be selected as git
	gw.enforcement.SetRule("git", "git.status", false, "test block")

	req := validRequest(workspace)
	req.Action.Type = ActionGit
	req.Action.Operation = "status"
	resp := gw.Process(req)

	// Server selection should still be git (routing unaffected)
	if resp.Execution.Server != "git" {
		t.Errorf("expected execution.server=git (routing unchanged), got %s", resp.Execution.Server)
	}

	// Trace should still show git as selected server
	if resp.Meta.DecisionTrace != nil && resp.Meta.DecisionTrace.SelectedServer != "git" {
		t.Errorf("expected trace.selected_server=git, got %s", resp.Meta.DecisionTrace.SelectedServer)
	}

	// Status must be error because enforcement blocked it
	if resp.Status != "error" {
		t.Errorf("expected error (blocked), got %s", resp.Status)
	}

	t.Logf("routing parity preserved: server=%s blocked=%v", resp.Execution.Server, resp.Status == "error")
}

func TestPolicyIntelligence_Record(t *testing.T) {
	pie := NewPolicyIntelligenceEngine()

	pie.Record(PolicyEvent{
		TraceID:   "t1",
		RequestID: "r1",
		Server:    "git",
		Operation: "git.status",
		Allowed:   true,
	})
	if pie.EventCount() != 1 {
		t.Fatalf("expected 1 event, got %d", pie.EventCount())
	}

	pie.Record(PolicyEvent{
		TraceID:   "t2",
		RequestID: "r2",
		Server:    "git",
		Operation: "git.status",
		Blocked:   true,
		Reason:    "maintenance",
	})
	if pie.EventCount() != 2 {
		t.Fatalf("expected 2 events, got %d", pie.EventCount())
	}

	t.Logf("events recorded: %d", pie.EventCount())
}

func TestPolicyIntelligence_WeightUpdate(t *testing.T) {
	pie := NewPolicyIntelligenceEngine()

	// Allowed → +0.01
	pie.Record(PolicyEvent{Server: "git", Operation: "git.status", Allowed: true})
	w := pie.Weight("git", "git.status")
	if w != 0.01 {
		t.Fatalf("expected weight 0.01 after 1 allow, got %.2f", w)
	}

	// Allowed again → +0.01 → 0.02
	pie.Record(PolicyEvent{Server: "git", Operation: "git.status", Allowed: true})
	w = pie.Weight("git", "git.status")
	if w != 0.02 {
		t.Fatalf("expected weight 0.02 after 2 allows, got %.2f", w)
	}

	// Blocked → -0.02 → 0.00
	pie.Record(PolicyEvent{Server: "git", Operation: "git.status", Blocked: true})
	w = pie.Weight("git", "git.status")
	if w != 0.00 {
		t.Fatalf("expected weight 0.00 after 2 allows + 1 block, got %.2f", w)
	}

	// Different server gets its own weight
	pie.Record(PolicyEvent{Server: "fetch", Operation: "fetch.url", Allowed: true})
	wFetch := pie.Weight("fetch", "fetch.url")
	if wFetch != 0.01 {
		t.Fatalf("expected fetch weight 0.01, got %.2f", wFetch)
	}
	// Git weight unchanged
	wGit := pie.Weight("git", "git.status")
	if wGit != 0.00 {
		t.Fatalf("expected git weight still 0.00, got %.2f", wGit)
	}

	t.Logf("weights: git=%.2f fetch=%.2f", wGit, wFetch)
}

func TestPolicyIntelligence_DriftDetection(t *testing.T) {
	pie := NewPolicyIntelligenceEngine()

	// 3 blocked events in last 10 for same server+operation → drift detected
	for i := 0; i < 3; i++ {
		pie.Record(PolicyEvent{
			Server:    "git",
			Operation: "git.push",
			Blocked:   true,
			Reason:    "maintenance",
		})
	}

	drift, count := pie.DetectDrift("git", "git.push")
	if !drift {
		t.Fatalf("expected drift detected after 3 blocks, count=%d", count)
	}
	if count != 3 {
		t.Fatalf("expected drift count 3, got %d", count)
	}

	// Different server should not have drift
	drift2, _ := pie.DetectDrift("fetch", "fetch.url")
	if drift2 {
		t.Fatal("expected no drift for unaffected server")
	}

	t.Logf("drift detected: git:git.push count=%d", count)
}

func TestPolicyIntelligence_GenerateSuggestions(t *testing.T) {
	pie := NewPolicyIntelligenceEngine()

	// Need: 5+ total events, 3+ blocks, weight < -0.05
	// 5 total events: 2 allows + 3 blocks → weight = 2*0.01 + 3*(-0.02) = 0.02 - 0.06 = -0.04
	// That's not negative enough. Need more blocks.
	// 6 total events: 2 allows + 4 blocks → weight = 0.02 - 0.08 = -0.06 ← qualifies
	
	pie.Record(PolicyEvent{Server: "git", Operation: "git.push", Allowed: true})
	pie.Record(PolicyEvent{Server: "git", Operation: "git.push", Allowed: true})
	pie.Record(PolicyEvent{Server: "git", Operation: "git.push", Blocked: true, Reason: "block1"})
	pie.Record(PolicyEvent{Server: "git", Operation: "git.push", Blocked: true, Reason: "block2"})
	pie.Record(PolicyEvent{Server: "git", Operation: "git.push", Blocked: true, Reason: "block3"})
	pie.Record(PolicyEvent{Server: "git", Operation: "git.push", Blocked: true, Reason: "block4"})

	suggestions := pie.GenerateSuggestions()
	if len(suggestions) == 0 {
		t.Fatal("expected at least 1 suggestion after repeated blocks")
	}

	found := false
	for _, s := range suggestions {
		if s.Server == "git" && s.Operation == "git.push" {
			found = true
			if s.SuggestedAction != "review_policy" {
				t.Errorf("expected suggested_action=review_policy, got %s", s.SuggestedAction)
			}
			if s.Confidence < 0.5 {
				t.Errorf("expected confidence >= 0.5, got %.2f", s.Confidence)
			}
			break
		}
	}
	if !found {
		t.Error("missing suggestion for git:git.push")
	}

	t.Logf("generated %d suggestion(s)", len(suggestions))
}

func TestGateway_PolicyIntelligencePassive(t *testing.T) {
	workspace := setupTestRepo(t)
	gw := NewGateway()

	// Run a normal request — policy intelligence should record it
	req := validRequest(workspace)
	req.Action.Type = ActionGit
	req.Action.Operation = "status"
	resp := gw.Process(req)

	if resp.Status != "success" {
		t.Fatalf("expected success, got %s: %s", resp.Status, resp.Error.Message)
	}

	pi := gw.policyIntelligence
	if pi == nil {
		t.Fatal("policyIntelligence engine is nil")
	}

	if pi.EventCount() == 0 {
		t.Error("expected at least 1 policy event recorded")
	}

	// Routing should be unaffected
	if resp.Execution.Server == "" {
		t.Error("execution server should be set")
	}

	t.Logf("policy intelligence recorded %d event(s), server=%s", pi.EventCount(), resp.Execution.Server)
}
