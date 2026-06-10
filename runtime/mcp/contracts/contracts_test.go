package contracts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

// ---- Backpressure Tests ----

func TestBackpressure_AcquireRelease(t *testing.T) {
	bp := NewBackpressureModel(DefaultBackpressureConfig())

	if err := bp.Acquire("s1", "filesystem"); err != nil {
		t.Fatalf("expected acquire to succeed: %v", err)
	}
	if bp.ActiveCount() != 1 {
		t.Fatalf("expected active=1, got %d", bp.ActiveCount())
	}
	if bp.SessionCount("s1") != 1 {
		t.Fatalf("expected session count=1, got %d", bp.SessionCount("s1"))
	}

	bp.Release("s1", "filesystem")
	if bp.ActiveCount() != 0 {
		t.Fatalf("expected active=0 after release, got %d", bp.ActiveCount())
	}
}

func TestBackpressure_SessionLimit(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	cfg.MaxQueuePerSession = 2
	bp := NewBackpressureModel(cfg)

	for i := 0; i < 2; i++ {
		if err := bp.Acquire("s1", "filesystem"); err != nil {
			t.Fatalf("attempt %d: expected accept: %v", i, err)
		}
	}

	if err := bp.Acquire("s1", "filesystem"); err == nil {
		t.Fatal("expected rejection when exceeding per-session limit")
	} else {
		t.Logf("session limit rejection: %v", err)
	}
}

func TestBackpressure_GlobalLimit(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	cfg.MaxQueuePerSession = 100
	cfg.MaxQueueTotal = 3
	bp := NewBackpressureModel(cfg)

	for i := 0; i < 3; i++ {
		sid := fmt.Sprintf("s%d", i)
		if err := bp.Acquire(sid, "filesystem"); err != nil {
			t.Fatalf("attempt %d: expected accept: %v", i, err)
		}
	}

	if err := bp.Acquire("s_extra", "filesystem"); err == nil {
		t.Fatal("expected rejection when exceeding global limit")
	} else {
		t.Logf("global limit rejection: %v", err)
	}
}

func TestBackpressure_Saturation(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	cfg.MaxQueuePerSession = 100
	cfg.MaxQueueTotal = 10
	cfg.SoftRejectionPct = 0.5
	bp := NewBackpressureModel(cfg)

	if bp.IsSaturated() {
		t.Fatal("should not be saturated at 0 active")
	}

	for i := 0; i < 5; i++ {
		bp.Acquire(fmt.Sprintf("s%d", i), "filesystem")
	}

	if !bp.IsSaturated() {
		t.Fatal("should be saturated at 5/10 (50% >= 50% threshold)")
	}
}

func TestBackpressure_ReleaseNonExistent(t *testing.T) {
	bp := NewBackpressureModel(DefaultBackpressureConfig())
	bp.Release("nonexistent", "filesystem")
	bp.Release("s1", "nonexistent_tool")
}

func TestBackpressure_ToolThrottle(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	cfg.MaxQueuePerSession = 100
	cfg.MaxQueueTotal = 100
	cfg.ToolThrottle = map[string]ToolThrottleConfig{
		"slow_tool": {MaxConcurrent: 2},
	}
	bp := NewBackpressureModel(cfg)

	for i := 0; i < 2; i++ {
		if err := bp.Acquire("s1", "slow_tool"); err != nil {
			t.Fatalf("attempt %d: expected accept: %v", i, err)
		}
	}

	if err := bp.Acquire("s1", "slow_tool"); err == nil {
		t.Fatal("expected tool throttle rejection")
	}
}

func TestBackpressure_ConcurrentSessions(t *testing.T) {
	cfg := DefaultBackpressureConfig()
	cfg.MaxQueuePerSession = 2
	cfg.MaxQueueTotal = 20
	bp := NewBackpressureModel(cfg)

	var wg sync.WaitGroup
	errs := make(chan error, 100)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			sid := fmt.Sprintf("s%d", id%5)
			if err := bp.Acquire(sid, "filesystem"); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	rejected := 0
	for range errs {
		rejected++
	}
	t.Logf("concurrent backpressure: %d rejected, %d active", rejected, bp.ActiveCount())
}

// ---- Circuit Breaker Tests ----

type fakeAdapter struct {
	name    string
	fail    bool
	latency time.Duration
}

func (f *fakeAdapter) Name() string { return f.name }

func (f *fakeAdapter) Execute(_ context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	if f.latency > 0 {
		time.Sleep(f.latency)
	}
	if f.fail {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "fake failure"}, errors.New("fake failure")
	}
	return types.MCPResponse{ID: req.ID, Success: true}, nil
}

func TestCircuitBreaker_ClosedToOpen(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 3
	cfg.RecoveryTimeout = 50 * time.Millisecond
	inner := &fakeAdapter{name: "test", fail: true}
	cb := NewCircuitBreaker(inner, cfg)

	if cb.State() != StateClosed {
		t.Fatalf("expected CLOSED initially, got %s", cb.State())
	}

	for i := 0; i < 3; i++ {
		cb.Execute(context.Background(), types.MCPRequest{ID: fmt.Sprintf("r%d", i)})
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN after 3 failures, got %s", cb.State())
	}
	t.Logf("circuit opened after 3 failures")
}

func TestCircuitBreaker_OpenRejects(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 2
	cfg.RecoveryTimeout = 1 * time.Hour
	inner := &fakeAdapter{name: "test", fail: true}
	cb := NewCircuitBreaker(inner, cfg)

	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), types.MCPRequest{ID: fmt.Sprintf("r%d", i)})
	}

	resp, err := cb.Execute(context.Background(), types.MCPRequest{ID: "rejected"})
	if err == nil {
		t.Fatal("expected OPEN rejection")
	}
	if resp.Success {
		t.Fatal("expected unsuccessful response on OPEN")
	}
	t.Logf("open rejection: %s", resp.Error)
}

func TestCircuitBreaker_OpenToHalfOpenToClosed(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 2
	cfg.RecoveryTimeout = 50 * time.Millisecond
	inner := &fakeAdapter{name: "test", fail: true}
	cb := NewCircuitBreaker(inner, cfg)

	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), types.MCPRequest{ID: fmt.Sprintf("r%d", i)})
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN, got %s", cb.State())
	}

	time.Sleep(60 * time.Millisecond)

	cb.inner = &fakeAdapter{name: "test", fail: false}

	resp, err := cb.Execute(context.Background(), types.MCPRequest{ID: "probe"})
	if err != nil {
		t.Fatalf("expected HALF_OPEN probe to succeed: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected successful probe response")
	}

	if cb.State() != StateClosed {
		t.Fatalf("expected CLOSED after successful probe, got %s", cb.State())
	}
	t.Logf("circuit recovered: OPEN → HALF_OPEN → CLOSED")
}

func TestCircuitBreaker_LatencySpike(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 2
	cfg.LatencyThreshold = 10 * time.Millisecond
	cfg.RecoveryTimeout = 1 * time.Hour
	inner := &fakeAdapter{name: "test", fail: false, latency: 50 * time.Millisecond}
	cb := NewCircuitBreaker(inner, cfg)

	for i := 0; i < 2; i++ {
		cb.Execute(context.Background(), types.MCPRequest{ID: fmt.Sprintf("r%d", i)})
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN after latency spike, got %s", cb.State())
	}
	t.Logf("circuit opened due to latency spike")
}

func TestCircuitBreaker_HalfOpenFailedProbeReopens(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 1
	cfg.RecoveryTimeout = 10 * time.Millisecond
	inner := &fakeAdapter{name: "test", fail: true}
	cb := NewCircuitBreaker(inner, cfg)

	cb.Execute(context.Background(), types.MCPRequest{ID: "r1"})
	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN, got %s", cb.State())
	}

	time.Sleep(20 * time.Millisecond)

	resp, err := cb.Execute(context.Background(), types.MCPRequest{ID: "p1"})
	if err == nil {
		t.Fatal("expected probe failure")
	}
	if resp.Success {
		t.Fatal("expected unsuccessful probe")
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN after failed probe, got %s", cb.State())
	}
	t.Logf("failed half-open probe re-opened circuit correctly")
}

// ---- Retry Budget Tests ----

func TestRetryBudget_Basic(t *testing.T) {
	cfg := DefaultRetryBudgetConfig()
	cfg.TotalBudget = 3
	cfg.PerToolCap = 3
	rb := NewRetryBudget(cfg)

	for i := 0; i < 3; i++ {
		if !rb.TryConsume("filesystem") {
			t.Fatalf("attempt %d: expected budget allow", i)
		}
	}

	if rb.TryConsume("filesystem") {
		t.Fatal("expected budget exhausted")
	}

	if rb.Remaining() != 0 {
		t.Fatalf("expected 0 remaining, got %d", rb.Remaining())
	}
}

func TestRetryBudget_PerToolCap(t *testing.T) {
	cfg := DefaultRetryBudgetConfig()
	cfg.TotalBudget = 100
	cfg.PerToolCap = 2
	rb := NewRetryBudget(cfg)

	for i := 0; i < 2; i++ {
		if !rb.TryConsume("slow_tool") {
			t.Fatalf("attempt %d: expected allow", i)
		}
	}

	if rb.TryConsume("slow_tool") {
		t.Fatal("expected per-tool cap exceeded")
	}

	if !rb.TryConsume("other_tool") {
		t.Fatal("expected different tool to be allowed")
	}
}

func TestRetryBudget_Context(t *testing.T) {
	ctx := context.Background()
	budget := NewRetryBudget(DefaultRetryBudgetConfig())

	ctx = WithRetryBudget(ctx, budget)

	got := GetRetryBudget(ctx)
	if got == nil {
		t.Fatal("expected budget from context")
	}
	if got.Remaining() != 3 {
		t.Fatalf("expected 3 remaining, got %d", got.Remaining())
	}
}

func TestRetryBudget_NoBudgetInContext(t *testing.T) {
	ctx := context.Background()
	got := GetRetryBudget(ctx)
	if got != nil {
		t.Fatal("expected nil budget for context without budget")
	}
}

func TestRetryBudget_Concurrent(t *testing.T) {
	cfg := DefaultRetryBudgetConfig()
	cfg.TotalBudget = 10
	cfg.PerToolCap = 10
	rb := NewRetryBudget(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rb.TryConsume("filesystem")
		}()
	}
	wg.Wait()

	if rb.Remaining() != 0 {
		t.Fatalf("expected 0 remaining after concurrent consumption, got %d", rb.Remaining())
	}
}

// ---- RetryAdapter + Budget Integration ----

func TestRetryAdapter_BudgetExhaustion(t *testing.T) {
	cfg := DefaultRetryBudgetConfig()
	cfg.TotalBudget = 1
	cfg.PerToolCap = 1

	inner := &fakeAdapter{name: "failing", fail: true}
	retryCfg := DefaultRetryConfig()
	retryCfg.MaxRetries = 5
	ra := NewRetryAdapter(inner, retryCfg)

	budget := NewRetryBudget(cfg)
	ctx := WithRetryBudget(context.Background(), budget)

	resp, _ := ra.Execute(ctx, types.MCPRequest{ID: "r1", Tool: "filesystem"})
	if resp.Success {
		t.Fatal("expected failure after budget exhaustion")
	}
	t.Logf("retry budget exhausted: %s", resp.Error)
}

func TestRetryAdapter_WithBudget_Success(t *testing.T) {
	cfg := DefaultRetryBudgetConfig()
	cfg.TotalBudget = 3

	inner := &fakeAdapter{name: "works", fail: false}
	ra := NewRetryAdapter(inner, DefaultRetryConfig())

	budget := NewRetryBudget(cfg)
	ctx := WithRetryBudget(context.Background(), budget)

	resp, err := ra.Execute(ctx, types.MCPRequest{ID: "r1", Tool: "filesystem"})
	if err != nil {
		t.Fatalf("expected success: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success")
	}
}

// ---- RetryAdapter Backoff + Jitter ----

func TestJitteredDelay(t *testing.T) {
	base := 100 * time.Millisecond

	for attempt := 0; attempt < 3; attempt++ {
		d1 := jitteredDelay(base, attempt, 2*time.Second)
		d2 := jitteredDelay(base, attempt, 2*time.Second)

		if d1 < base {
			t.Fatalf("attempt %d: delay %v < base %v", attempt, d1, base)
		}
		if d1 > 2*time.Second {
			t.Fatalf("attempt %d: delay %v > max", attempt, d1)
		}

		if d1 == d2 {
			t.Logf("attempt %d: d1 == d2 == %v (low probability collision ok)", attempt, d1)
		} else {
			t.Logf("attempt %d: d1=%v d2=%v (jitter difference)", attempt, d1, d2)
		}
	}
}

func TestClampDelay(t *testing.T) {
	tests := []struct {
		d    time.Duration
		min  time.Duration
		max  time.Duration
		want time.Duration
	}{
		{50 * time.Millisecond, 100 * time.Millisecond, 1 * time.Second, 100 * time.Millisecond},
		{500 * time.Millisecond, 100 * time.Millisecond, 200 * time.Millisecond, 200 * time.Millisecond},
		{300 * time.Millisecond, 100 * time.Millisecond, 1 * time.Second, 300 * time.Millisecond},
	}
	for _, tc := range tests {
		got := clampDelay(tc.d, tc.min, tc.max)
		if got != tc.want {
			t.Fatalf("clampDelay(%v, %v, %v) = %v, want %v", tc.d, tc.min, tc.max, got, tc.want)
		}
	}
}

// ---- CircuitBreaker Concurrent Test ----

func TestCircuitBreaker_Concurrent(t *testing.T) {
	cfg := DefaultCircuitBreakerConfig()
	cfg.FailureThreshold = 3
	cfg.RecoveryTimeout = 1 * time.Hour
	inner := &fakeAdapter{name: "stress", fail: true}
	cb := NewCircuitBreaker(inner, cfg)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cb.Execute(context.Background(), types.MCPRequest{ID: fmt.Sprintf("r%d", id)})
		}(i)
	}
	wg.Wait()

	if cb.State() != StateOpen {
		t.Fatalf("expected OPEN after concurrent failures, got %s", cb.State())
	}
	t.Logf("concurrent circuit breaker opened correctly")
}

// ---- Backpressure + Circuit Breaker Integration ----

func TestFullChain_BackpressureAndCircuitBreaker(t *testing.T) {
	bp := NewBackpressureModel(DefaultBackpressureConfig())
	failAdapter := &fakeAdapter{name: "failing_tool", fail: true}
	cb := NewCircuitBreaker(failAdapter, DefaultCircuitBreakerConfig())

	for i := 0; i < 10; i++ {
		sid := fmt.Sprintf("s%d", i%3)
		if err := bp.Acquire(sid, "failing_tool"); err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		cb.Execute(context.Background(), types.MCPRequest{ID: fmt.Sprintf("r%d", i)})
		bp.Release(sid, "failing_tool")
	}

	if bp.ActiveCount() != 0 {
		t.Fatalf("expected all released, got %d active", bp.ActiveCount())
	}

	resp, err := cb.Execute(context.Background(), types.MCPRequest{ID: "final"})
	if err == nil {
		t.Fatal("expected circuit breaker open")
	}
	if resp.Success {
		t.Fatal("expected failure after circuit open")
	}
	t.Logf("backpressure + circuit breaker integration: circuit=%s error=%s", cb.State(), resp.Error)
}
