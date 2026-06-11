package mcpv2

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGitAdapter_Status(t *testing.T) {
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "t@t.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "t").Run()
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("data"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "init").Run()

	srv := &GitServer{}
	ctx := MCPContext{Workspace: struct {
		Path string "json:\"path\""
		Repo string "json:\"repo\""
	}{Path: dir}}
	result, err := srv.Execute("git.status", nil, ctx)
	if err != nil {
		t.Fatalf("git status failed: %v", err)
	}
	data, _ := json.Marshal(result)
	if !containsJSON(data, "output") {
		t.Fatalf("expected output in result: %s", data)
	}
}

func TestGitAdapter_Commit(t *testing.T) {
	dir, err := os.MkdirTemp("", "git-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	exec.Command("git", "init", dir).Run()
	exec.Command("git", "-C", dir, "config", "user.email", "t@t.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "t").Run()

	srv := &GitServer{}
	ctx := MCPContext{Workspace: struct {
		Path string "json:\"path\""
		Repo string "json:\"repo\""
	}{Path: dir}}

	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package main"), 0644)
	result, err := srv.Execute("git.commit", map[string]any{"message": "first commit"}, ctx)
	if err != nil {
		t.Fatalf("git commit failed: %v", err)
	}
	data, _ := json.Marshal(result)
	if !containsJSON(data, "output") {
		t.Fatalf("expected output in result: %s", data)
	}
}

func TestFilesystemAdapter_ReadWrite(t *testing.T) {
	dir, err := os.MkdirTemp("", "fs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srv := &FilesystemServer{}
	ctx := MCPContext{Workspace: struct {
		Path string "json:\"path\""
		Repo string "json:\"repo\""
	}{Path: dir}}

	_, err = srv.Execute("filesystem.write", map[string]any{"path": "test.txt", "content": "hello"}, ctx)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("file not found after write: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected 'hello', got '%s'", data)
	}

	result, err := srv.Execute("filesystem.read", map[string]any{"path": "test.txt"}, ctx)
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "content") {
		t.Fatalf("expected content in read result: %s", r)
	}
}

func TestFilesystemAdapter_PathTraversal(t *testing.T) {
	dir, err := os.MkdirTemp("", "fs-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	srv := &FilesystemServer{}
	ctx := MCPContext{Workspace: struct {
		Path string "json:\"path\""
		Repo string "json:\"repo\""
	}{Path: dir}}

	_, err = srv.Execute("filesystem.read", map[string]any{"path": "../../etc/passwd"}, ctx)
	if err == nil {
		t.Fatal("expected path traversal error")
	}
}

func TestGitHubAdapter_AuthRequired(t *testing.T) {
	srv := NewGitHubServer()
	ctx := MCPContext{}

	_, err := srv.Execute("github.read", map[string]any{
		"owner": "test",
		"repo":  "test",
		"path":  "README.md",
	}, ctx)
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestMemoryAdapter_StoreRetrieve(t *testing.T) {
	srv := NewMemoryServer("")

	_, err := srv.Execute("memory.store", map[string]any{"key": "k1", "value": "v1"}, MCPContext{})
	if err != nil {
		t.Fatalf("store failed: %v", err)
	}

	result, err := srv.Execute("memory.retrieve", map[string]any{"key": "k1"}, MCPContext{})
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "v1") {
		t.Fatalf("expected 'v1' in result: %s", r)
	}
}

func TestMemoryAdapter_MissingKey(t *testing.T) {
	srv := NewMemoryServer("")
	_, err := srv.Execute("memory.retrieve", map[string]any{"key": "nonexistent"}, MCPContext{})
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestContext7Adapter_Deterministic(t *testing.T) {
	srv := NewContext7Server()

	r1, err := srv.Execute("context7.query", map[string]any{"key": "ctx-key"}, MCPContext{})
	if err != nil {
		t.Fatalf("first query failed: %v", err)
	}
	r2, _ := srv.Execute("context7.query", map[string]any{"key": "ctx-key"}, MCPContext{})
	if err != nil {
		t.Fatalf("second query failed: %v", err)
	}

	j1, _ := json.Marshal(r1)
	j2, _ := json.Marshal(r2)
	if string(j1) != string(j2) {
		t.Fatal("expected deterministic response for same key")
	}
}

func TestContext7Adapter_Resolve(t *testing.T) {
	srv := NewContext7Server()
	result, err := srv.Execute("context7.resolve", map[string]any{"key": "session"}, MCPContext{
		SessionID: "sess-1",
		TraceID:   "trace-1",
		Workspace: struct {
			Path string "json:\"path\""
			Repo string "json:\"repo\""
		}{Path: "/workspace"},
	})
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "sess-1") {
		t.Fatalf("expected session context: %s", r)
	}
}

func TestPostgresAdapter_QueryFallback(t *testing.T) {
	srv := NewPostgresAdapter()
	result, err := srv.Execute("postgres.query", map[string]any{"sql": "SELECT 1"}, MCPContext{})
	if err != nil {
		t.Fatalf("postgres query fallback failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "logged") {
		t.Fatalf("expected logged status: %s", r)
	}
}

func TestPostgresAdapter_ListTables(t *testing.T) {
	srv := NewPostgresAdapter()
	result, err := srv.Execute("postgres.list_tables", nil, MCPContext{})
	if err != nil {
		t.Fatalf("postgres list_tables failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "logged") {
		t.Fatalf("expected logged status: %s", r)
	}
}

func TestChromaAdapter_StoreFallback(t *testing.T) {
	clearEnv := clearChromaEnv(t)
	defer clearEnv()
	srv := NewChromaAdapter()
	result, err := srv.Execute("chroma.store", map[string]any{
		"id":       "doc-1",
		"document": "test document content",
	}, MCPContext{SessionID: "sess-1", TraceID: "tr-1"})
	if err != nil {
		t.Fatalf("chroma store failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "id") {
		t.Fatalf("expected result with id: %s", r)
	}
}

func TestChromaAdapter_QueryFallback(t *testing.T) {
	clearEnv := clearChromaEnv(t)
	defer clearEnv()
	srv := NewChromaAdapter()
	result, err := srv.Execute("chroma.query", map[string]any{"query": "test query"}, MCPContext{})
	if err != nil {
		t.Fatalf("chroma query failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "query") {
		t.Fatalf("expected result with query: %s", r)
	}
}

func TestChromaAdapter_ListCollections(t *testing.T) {
	clearEnv := clearChromaEnv(t)
	defer clearEnv()
	srv := NewChromaAdapter()
	result, err := srv.Execute("chroma.list_collections", nil, MCPContext{})
	if err != nil {
		t.Fatalf("chroma list_collections failed: %v", err)
	}
	r, _ := json.Marshal(result)
	if !containsJSON(r, "collections") || !containsJSON(r, "status") {
		t.Fatalf("expected collections result: %s", r)
	}
}

func TestPostgresAdapter_RequiresSQL(t *testing.T) {
	srv := NewPostgresAdapter()
	_, err := srv.Execute("postgres.query", map[string]any{}, MCPContext{})
	if err == nil {
		t.Fatal("expected error for missing sql")
	}
}

func TestChromaAdapter_RequiresID(t *testing.T) {
	clearEnv := clearChromaEnv(t)
	defer clearEnv()
	srv := NewChromaAdapter()
	_, err := srv.Execute("chroma.store", map[string]any{"document": "content"}, MCPContext{})
	if err == nil {
		t.Fatal("expected error for missing id")
	}
}

func clearChromaEnv(t *testing.T) func() {
	t.Helper()
	keys := []string{"CHROMA_API_KEY", "CHROMA_TENANT", "CHROMA_DATABASE", "CHROMA_HOST", "CHROMA_PORT", "CHROMA_URL"}
	saved := make(map[string]string)
	for _, k := range keys {
		saved[k] = os.Getenv(k)
		os.Unsetenv(k)
	}
	return func() {
		for k, v := range saved {
			if v != "" {
				os.Setenv(k, v)
			}
		}
	}
}

func containsJSON(data []byte, substr string) bool {
	return bytes.Contains(data, []byte(substr))
}
