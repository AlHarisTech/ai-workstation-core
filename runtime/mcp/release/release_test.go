package release

import (
	"context"
	"testing"
	"time"

	mcpobs "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/observability"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

func TestReleaseOrchestrator_MissingInput(t *testing.T) {
	var events []mcpobs.TraceEvent
	orch := NewReleaseOrchestrator("", "owner", "repo", func(ev mcpobs.TraceEvent) {
		events = append(events, ev)
	})

	req := types.MCPRequest{ID: "r1", Payload: map[string]any{}}
	resp, err := orch.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for missing input")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (start + failure), got %d", len(events))
	}
}

func TestReleaseOrchestrator_EmptyTokenFailsTagCreation(t *testing.T) {
	var events []mcpobs.TraceEvent
	orch := NewReleaseOrchestrator("", "AlHarisTech", "ai-workstation-core", func(ev mcpobs.TraceEvent) {
		events = append(events, ev)
	})

	req := types.MCPRequest{ID: "r1", Payload: map[string]any{
		"version":    "v0.0.0-test-nonexistent",
		"commit_sha": "abc123def4567890",
	}}
	resp, err := orch.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-existent commit SHA causes git tag to fail
	if resp.Success {
		t.Fatal("expected failure when tag creation fails")
	}
	if len(events) < 1 {
		t.Fatalf("expected at least 1 event, got %d", len(events))
	}
	t.Logf("got expected error: %s", resp.Error)
}

func TestReleaseOrchestrator_Name(t *testing.T) {
	orch := NewReleaseOrchestrator("token", "o", "r", nil)
	if orch.Name() != "mcp-release-orchestrator" {
		t.Fatalf("unexpected name: %s", orch.Name())
	}
}

func TestGitHubBridge_EmptyToken(t *testing.T) {
	bridge := NewGitHubBridge("", "owner", "repo")
	err := bridge.ValidateAccess()
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestGitHubBridge_CreateRelease_NoToken(t *testing.T) {
	bridge := NewGitHubBridge("", "owner", "repo")
	_, err := bridge.CreateRelease("v1.0.0", "abc123", "notes")
	if err == nil {
		t.Fatal("expected error for no token")
	}
}

// ---- Retry Tests ----

func TestComputeNextRetry_ZeroAttempts(t *testing.T) {
	d := ComputeNextRetry(0, "abc", "v1.0.0")
	if d != 0 {
		t.Fatalf("expected 0 delay for 0 attempts, got %v", d)
	}
}

func TestComputeNextRetry_Bounded(t *testing.T) {
	d := ComputeNextRetry(maxRetryAttempts+1, "abc", "v1.0.0")
	if d != 0 {
		t.Fatalf("expected 0 delay beyond max attempts, got %v", d)
	}
}

func TestComputeNextRetry_Exponential(t *testing.T) {
	d1 := ComputeNextRetry(1, "abc", "v1.0.0")
	d2 := ComputeNextRetry(2, "abc", "v1.0.0")
	d3 := ComputeNextRetry(3, "abc", "v1.0.0")
	if d1 <= 0 || d2 <= 0 || d3 <= 0 {
		t.Fatal("all delays must be positive")
	}
	if d2 < d1 {
		t.Fatal("expected exponential growth: attempt 2 must be >= attempt 1")
	}
	t.Logf("delays: 1=%v, 2=%v, 3=%v", d1, d2, d3)
}

func TestComputeNextRetry_MaxBound(t *testing.T) {
	// At attempt ~8, should cap at maxBackoffSeconds
	d := ComputeNextRetry(8, "abc", "v1.0.0")
	maxDur := time.Duration(maxBackoffSeconds * float64(time.Second))
	if d > maxDur {
		t.Fatalf("expected cap at %v, got %v", maxDur, d)
	}
}

func TestDeterministicJitter_Stable(t *testing.T) {
	j1 := deterministicJitter("abc123", "v1.0.0", 1, 0.2)
	j2 := deterministicJitter("abc123", "v1.0.0", 1, 0.2)
	if j1 != j2 {
		t.Fatal("deterministic jitter must produce same output for same input")
	}
}

func TestDeterministicJitter_Range(t *testing.T) {
	j := deterministicJitter("abc", "v1.0.0", 1, 0.2)
	if j < 0 || j > 0.2 {
		t.Fatalf("jitter out of range [0, 0.2]: %f", j)
	}
}

func TestAdvanceRetryState_Finalized(t *testing.T) {
	rs := NewRetryState("abc", "v1.0.0")
	for i := 0; i < maxRetryAttempts; i++ {
		rs = AdvanceRetryState(rs, "abc", "v1.0.0")
	}
	if !rs.Finalized {
		t.Fatal("expected finalized after max attempts")
	}
}

func TestIsRetryDue(t *testing.T) {
	rs := RetryState{Attempts: 0, NextRetryAt: 0, Finalized: false}
	if !IsRetryDue(rs) {
		t.Fatal("expected retry due when next_retry_at is 0")
	}
}

func TestIsRetryDue_Finalized(t *testing.T) {
	rs := RetryState{Finalized: true, NextRetryAt: 0}
	if IsRetryDue(rs) {
		t.Fatal("expected no retry due when finalized")
	}
}

// ---- Queue Tests ----

func TestReleaseQueue_Enqueue(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	input := ReleaseInput{Version: "v1.0.0", CommitSHA: "abc123def456"}
	entry := q.Enqueue(input, "owner", "repo")
	if entry.Status != StatusPendingExternal {
		t.Fatalf("expected PENDING_EXTERNAL, got %s", entry.Status)
	}
	if q.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", q.PendingCount())
	}
}

func TestReleaseQueue_Dequeue(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	input := ReleaseInput{Version: "v1.0.0", CommitSHA: "abc123def456"}
	q.Enqueue(input, "owner", "repo")

	entry, ok := q.Dequeue()
	if !ok {
		t.Fatal("expected dequeue to return entry")
	}
	if entry.Status != StatusPublishing {
		t.Fatalf("expected PUBLISHING, got %s", entry.Status)
	}
}

func TestReleaseQueue_Complete_Success(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	input := ReleaseInput{Version: "v1.0.0", CommitSHA: "abc123def456"}
	q.Enqueue(input, "owner", "repo")

	entry, _ := q.Dequeue()
	q.Complete(entry.ReleaseID, true, "")

	_, ok := q.Dequeue()
	if ok {
		t.Fatal("expected no more dequeues after success")
	}
}

func TestReleaseQueue_Complete_FailureRetry(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	input := ReleaseInput{Version: "v1.0.0", CommitSHA: "abc123def456"}
	q.Enqueue(input, "owner", "repo")

	entry, _ := q.Dequeue()
	q.Complete(entry.ReleaseID, false, "network error")

	// Should be QUEUED for retry, not completed
	snap := q.Snapshot()
	if snap[0].Status != StatusQueued {
		t.Fatalf("expected QUEUED for retry, got %s", snap[0].Status)
	}
}

func TestReleaseQueue_JSON(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	q.Enqueue(ReleaseInput{Version: "v1.0.0", CommitSHA: "abcdef1234567890"}, "o", "r")
	json := q.JSON()
	if len(json) == 0 {
		t.Fatal("expected non-empty JSON output")
	}
}

func TestReleaseQueue_EmptyDequeue(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	_, ok := q.Dequeue()
	if ok {
		t.Fatal("expected no dequeue from empty queue")
	}
}

func TestReleaseQueue_PendingCount(t *testing.T) {
	q := NewReleaseQueue(nil, "")
	if q.PendingCount() != 0 {
		t.Fatal("expected 0 pending count for empty queue")
	}
}

// ---- Idempotency Tests ----

func TestIdempotencyKey_Deterministic(t *testing.T) {
	k1 := IdempotencyKey("abc123", "v1.0.0")
	k2 := IdempotencyKey("abc123", "v1.0.0")
	if k1 != k2 {
		t.Fatal("idempotency key must be deterministic")
	}
}

func TestIdempotencyKey_DifferentInputs(t *testing.T) {
	k1 := IdempotencyKey("abc123", "v1.0.0")
	k2 := IdempotencyKey("def456", "v1.0.0")
	if k1 == k2 {
		t.Fatal("different inputs must produce different keys")
	}
}

func TestIdempotencyStore_Exists(t *testing.T) {
	store := NewIdempotencyStore()
	key := IdempotencyKey("abc", "v1.0.0")
	if store.Exists(key) {
		t.Fatal("expected not exists before mark")
	}
	store.Mark(key)
	if !store.Exists(key) {
		t.Fatal("expected exists after mark")
	}
}

func TestIdempotencyStore_Snapshot(t *testing.T) {
	store := NewIdempotencyStore()
	store.Mark("k1")
	store.Mark("k2")
	snap := store.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(snap))
	}
}

// ---- Persistence Tests ----

func TestPersistLoadQueue_RoundTrip(t *testing.T) {
	path := t.TempDir() + "/test_queue.json"
	entries := []ReleaseEntry{
		{ReleaseID: "r1", CommitSHA: "abc", Version: "v1.0.0", Status: StatusPendingExternal},
		{ReleaseID: "r2", CommitSHA: "def", Version: "v1.1.0", Status: StatusCompleted},
	}
	if err := PersistQueue(entries, path); err != nil {
		t.Fatalf("persist failed: %v", err)
	}
	loaded, err := LoadQueue(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(loaded))
	}
	if loaded[0].ReleaseID != "r1" {
		t.Fatalf("expected r1, got %s", loaded[0].ReleaseID)
	}
}

func TestPersistLoadQueue_NonExistent(t *testing.T) {
	entries, err := LoadQueue("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("expected no error for non-existent file: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty slice, got %d", len(entries))
	}
}

func TestPersistLoadIdempotencyStore_RoundTrip(t *testing.T) {
	path := t.TempDir() + "/test_idempotency.json"
	if err := PersistCompletedEntry("test_key_1", path); err != nil {
		t.Fatalf("persist failed: %v", err)
	}
	loaded, err := LoadIdempotencyStore(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if !loaded["test_key_1"] {
		t.Fatal("expected test_key_1 to be marked")
	}
}

func TestPersistLoadIdempotencyStore_NonExistent(t *testing.T) {
	store, err := LoadIdempotencyStore("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("expected no error for non-existent file: %v", err)
	}
	if len(store) != 0 {
		t.Fatalf("expected empty store, got %d", len(store))
	}
}

func TestReleaseQueue_RecoveryFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/recovery_test.json"
	entries := []ReleaseEntry{
		{ReleaseID: "r1", CommitSHA: "abc123", Version: "v1.0.0", Status: StatusPendingExternal},
		{ReleaseID: "r2", CommitSHA: "def456", Version: "v1.1.0", Status: StatusCompleted},
	}
	if err := PersistQueue(entries, path); err != nil {
		t.Fatalf("persist failed: %v", err)
	}

	q := NewReleaseQueue(nil, path)
	// r1 should be recovered as pending
	entry, ok := q.Dequeue()
	if !ok {
		t.Fatal("expected recovered entry to be dequeued")
	}
	if entry.ReleaseID != "r1" {
		t.Fatalf("expected r1, got %s", entry.ReleaseID)
	}
}

func TestReleaseQueue_PersistAfterMutation(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/mutation_test.json"

	q := NewReleaseQueue(nil, path)
	q.Enqueue(ReleaseInput{Version: "v1.0.0", CommitSHA: "abcdef123456"}, "o", "r")

	// Verify persisted
	loaded, err := LoadQueue(path)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("expected 1 persisted entry, got %d", len(loaded))
	}
	if loaded[0].Version != "v1.0.0" {
		t.Fatalf("expected v1.0.0, got %s", loaded[0].Version)
	}
}
