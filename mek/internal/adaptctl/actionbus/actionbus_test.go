package actionbus

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/feedback"
	"github.com/anomalyco/mek/internal/adaptctl/signal"
)

func makeAction(id string, t feedback.ActionType, issuedAt time.Time, ttl time.Duration) feedback.Action {
	return feedback.Action{
		ID:       id,
		Type:     t,
		SignalID: "sig-1",
		RuleID:   "R01",
		Target:   "app://test",
		IssuedAt: issuedAt,
		TTL:      ttl,
	}
}

// ─── Basic Delivery ───

func TestDeliverSingleAction(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 delivered, got %d", n)
	}
	if len(ep.Delivered) != 1 {
		t.Errorf("expected 1 action at endpoint, got %d", len(ep.Delivered))
	}
}

func TestDeliverMultipleActions(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now().Add(-2*time.Second), 0),
		makeAction("a2", feedback.Notify, time.Now().Add(-1*time.Second), 0),
		makeAction("a3", feedback.Notify, time.Now(), 0),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("expected 3 delivered, got %d", n)
	}

	// Best-effort ordering: should be a1, a2, a3
	for i, a := range ep.Delivered {
		expected := fmt.Sprintf("a%d", i+1)
		if a.ID != expected {
			t.Errorf("ordering violation at %d: expected %s, got %s", i, expected, a.ID)
		}
	}
}

// ─── Retry ───

func TestRetry_SucceedsAfterFailure(t *testing.T) {
	ep := &FailingEndpoint{FailCount: 2}
	cfg := DeliveryConfig{MaxAttempts: 3, RetryDelay: time.Millisecond}
	bus := New(ep, cfg)

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("expected 1 delivered (after retry), got %d", n)
	}
}

func TestRetry_Exhausted(t *testing.T) {
	ep := &FailingEndpoint{FailCount: 10}
	cfg := DeliveryConfig{MaxAttempts: 2, RetryDelay: time.Millisecond}
	bus := New(ep, cfg)

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err == nil {
		t.Error("expected error after retry exhaustion")
	}
	if n != 0 {
		t.Errorf("expected 0 delivered, got %d", n)
	}
}

// ─── TTL Expiration ───

func TestTTL_ExpiredActionDropped(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	// Action issued 10 seconds ago with 5 second TTL
	actions := []feedback.Action{
		makeAction("expired", feedback.Notify, time.Now().Add(-10*time.Second), 5*time.Second),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("expired action should not be delivered, got %d", n)
	}
	if len(ep.Delivered) != 0 {
		t.Error("expired action should not reach endpoint")
	}
}

func TestTTL_ValidActionDelivered(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	// Action issued now with 10 second TTL
	actions := []feedback.Action{
		makeAction("valid", feedback.Notify, time.Now(), 10*time.Second),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("valid action should be delivered, got %d", n)
	}
}

func TestTTL_ZeroNeverExpires(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	// Action issued 1 hour ago with zero TTL (never expires)
	actions := []feedback.Action{
		makeAction("eternal", feedback.Notify, time.Now().Add(-time.Hour), 0),
	}

	n, err := bus.Publish(context.Background(), actions)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("zero TTL should never expire, got %d delivered", n)
	}
}

// ─── Idempotency ───

func TestIdempotency_DuplicateDelivery(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
	}

	// First publish
	n1, _ := bus.Publish(context.Background(), actions)
	if n1 != 1 {
		t.Fatalf("first publish: expected 1, got %d", n1)
	}

	// Second publish — same action ID, should be skipped
	n2, _ := bus.Publish(context.Background(), actions)
	if n2 != 0 {
		t.Errorf("duplicate publish should deliver 0, got %d", n2)
	}

	if len(ep.Delivered) != 1 {
		t.Errorf("endpoint should have exactly 1 action, got %d", len(ep.Delivered))
	}
}

func TestIdempotency_DuplicateInSameBatch(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	// Same action ID twice in the same batch (simulated retry scenario)
	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
		makeAction("a1", feedback.Notify, time.Now().Add(time.Second), 0),
	}

	n, _ := bus.Publish(context.Background(), actions)
	if n != 1 {
		t.Errorf("duplicate in batch should deliver 1, got %d", n)
	}
	if len(ep.Delivered) != 1 {
		t.Errorf("endpoint should have 1 action, got %d", len(ep.Delivered))
	}
}

// ─── Edge Cases ───

func TestNilEndpoint(t *testing.T) {
	bus := New(nil, DefaultConfig())
	_, err := bus.Publish(context.Background(), []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
	})
	if err == nil {
		t.Error("expected error for nil endpoint")
	}
}

func TestEmptyActions(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	n, err := bus.Publish(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("empty actions should deliver 0, got %d", n)
	}
}

func TestContextCancellation(t *testing.T) {
	ep := &FailingEndpoint{FailCount: 100}
	cfg := DeliveryConfig{MaxAttempts: 10, RetryDelay: 10 * time.Millisecond}
	bus := New(ep, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
	}

	_, err := bus.Publish(ctx, actions)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

// ─── Multiple Signal Types ───

func TestMultipleSignalTypes(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	now := time.Now()
	actions := []feedback.Action{
		makeAction("halt", feedback.Halt, now, 0),
		makeAction("esc", feedback.Escalate, now.Add(time.Second), 0),
		makeAction("reex", feedback.Reexecute, now.Add(2*time.Second), 0),
		makeAction("notify", feedback.Notify, now.Add(3*time.Second), 0),
	}

	n, _ := bus.Publish(context.Background(), actions)
	if n != 4 {
		t.Errorf("expected 4 delivered, got %d", n)
	}
}

// ─── DeliveredCount ───

func TestDeliveredCount(t *testing.T) {
	ep := &NoopEndpoint{}
	bus := New(ep, DefaultConfig())

	actions := []feedback.Action{
		makeAction("a1", feedback.Notify, time.Now(), 0),
		makeAction("a2", feedback.Notify, time.Now(), 0),
	}
	bus.Publish(context.Background(), actions)

	if bus.DeliveredCount() != 2 {
		t.Errorf("expected 2 delivered, got %d", bus.DeliveredCount())
	}
}

// Ensure signal import is used
var _ = signal.StateAllPass
