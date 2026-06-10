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

func TestReleaseOrchestrator_AuthFailure(t *testing.T) {
	var events []mcpobs.TraceEvent
	orch := NewReleaseOrchestrator("", "AlHarisTech", "ai-workstation-core", func(ev mcpobs.TraceEvent) {
		events = append(events, ev)
	})

	req := types.MCPRequest{ID: "r1", Payload: map[string]any{
		"version":    "v1.0.0",
		"commit_sha": "abc123",
	}}
	resp, err := orch.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Success {
		t.Fatal("expected failure for invalid auth")
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (start + failure), got %d", len(events))
	}
	if resp.Error != "" {
		t.Logf("got expected error: %s", resp.Error)
	}
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
	q := NewReleaseQueue(nil)
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
	q := NewReleaseQueue(nil)
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
	q := NewReleaseQueue(nil)
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
	q := NewReleaseQueue(nil)
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
	q := NewReleaseQueue(nil)
	q.Enqueue(ReleaseInput{Version: "v1.0.0", CommitSHA: "abcdef1234567890"}, "o", "r")
	json := q.JSON()
	if len(json) == 0 {
		t.Fatal("expected non-empty JSON output")
	}
}

func TestReleaseQueue_EmptyDequeue(t *testing.T) {
	q := NewReleaseQueue(nil)
	_, ok := q.Dequeue()
	if ok {
		t.Fatal("expected no dequeue from empty queue")
	}
}

func TestReleaseQueue_PendingCount(t *testing.T) {
	q := NewReleaseQueue(nil)
	if q.PendingCount() != 0 {
		t.Fatal("expected 0 pending count for empty queue")
	}
}
