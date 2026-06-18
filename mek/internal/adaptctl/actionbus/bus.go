// Package actionbus delivers adaptive actions to application endpoints.
// It is a TRANSPORT LAYER ONLY — no business logic, no workflow orchestration.
//
// Guarantees (v1):
//   - At-least-once delivery (AB-001)
//   - Action.ID is idempotency key (AB-002)
//   - Best-effort ordering within a single publish batch
//   - TTL enforcement (AB-004)
//   - Process-local only (AB-003)
//
// Boundaries:
//   - Never imports MEK internals
//   - Never evaluates feedback rules
//   - Never modifies signal state
package actionbus

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anomalyco/mek/internal/adaptctl/feedback"
)

// Envelope wraps an action with delivery metadata.
type Envelope struct {
	ID        string
	Action    feedback.Action
	CreatedAt time.Time
	TTL       time.Duration
	Attempts  int
	Delivered bool
}

// Endpoint is the interface that applications implement to receive actions.
type Endpoint interface {
	// Deliver receives an action. The Action.ID serves as the idempotency key.
	// Returns nil on success, error if delivery failed (triggering retry).
	Deliver(ctx context.Context, action feedback.Action) error
}

// DeliveryConfig controls retry and TTL behavior.
type DeliveryConfig struct {
	MaxAttempts int
	RetryDelay  time.Duration
}

// DefaultConfig returns a reasonable v1 delivery configuration.
func DefaultConfig() DeliveryConfig {
	return DeliveryConfig{
		MaxAttempts: 3,
		RetryDelay:  100 * time.Millisecond,
	}
}

// Bus delivers actions to a single endpoint with at-least-once semantics.
type Bus struct {
	mu       sync.Mutex
	endpoint Endpoint
	config   DeliveryConfig
	// delivered tracks Action.IDs that have been successfully delivered
	// within this process lifetime (idempotency guard — AB-002).
	delivered map[string]time.Time
}

// New creates a Bus with the given endpoint and configuration.
func New(endpoint Endpoint, config DeliveryConfig) *Bus {
	return &Bus{
		endpoint:  endpoint,
		config:    config,
		delivered: make(map[string]time.Time),
	}
}

// Publish delivers a batch of actions to the endpoint.
// Actions are sorted by timestamp before delivery (best-effort ordering).
// Expired actions are silently dropped (AB-004).
// Already-delivered actions are skipped (idempotency — AB-002).
//
// Returns the number of actions successfully delivered.
func (b *Bus) Publish(ctx context.Context, actions []feedback.Action) (int, error) {
	if b.endpoint == nil {
		return 0, fmt.Errorf("actionbus: nil endpoint")
	}
	if len(actions) == 0 {
		return 0, nil
	}

	// Filter and sort
	valid := b.filterValid(actions)
	sort.Slice(valid, func(i, j int) bool {
		return valid[i].IssuedAt.Before(valid[j].IssuedAt)
	})

	delivered := 0
	var lastErr error

	for _, action := range valid {
		if err := b.deliverWithRetry(ctx, action); err != nil {
			lastErr = err
			continue
		}
		delivered++
	}

	if delivered == 0 && lastErr != nil {
		return 0, lastErr
	}

	return delivered, nil
}

// filterValid removes expired, already-delivered, and in-batch duplicate actions.
func (b *Bus) filterValid(actions []feedback.Action) []feedback.Action {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	var valid []feedback.Action
	seen := make(map[string]bool)

	for _, a := range actions {
		// AB-004: Expired actions must not be delivered.
		if isExpired(a.IssuedAt, a.TTL, now) {
			continue
		}
		// AB-002: Action.ID is idempotency key.
		if _, ok := b.delivered[a.ID]; ok {
			continue
		}
		// Deduplicate within the same batch.
		if seen[a.ID] {
			continue
		}
		seen[a.ID] = true
		valid = append(valid, a)
	}

	return valid
}

// deliverWithRetry attempts to deliver an action up to MaxAttempts times.
func (b *Bus) deliverWithRetry(ctx context.Context, action feedback.Action) error {
	var lastErr error

	for attempt := 0; attempt < b.config.MaxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(b.config.RetryDelay):
			}
		}

		err := b.endpoint.Deliver(ctx, action)
		if err == nil {
			b.markDelivered(action.ID)
			return nil
		}
		lastErr = err
	}

	return fmt.Errorf("delivery exhausted after %d attempts: %w", b.config.MaxAttempts, lastErr)
}

// markDelivered records a successfully delivered action ID.
func (b *Bus) markDelivered(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.delivered[id] = time.Now()
}

// isExpired checks if an action's TTL has elapsed.
func isExpired(issuedAt time.Time, ttl time.Duration, now time.Time) bool {
	if ttl <= 0 {
		return false // zero TTL = never expires
	}
	return now.After(issuedAt.Add(ttl))
}

// DeliveredCount returns the number of unique actions delivered.
func (b *Bus) DeliveredCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.delivered)
}

// ─── NoopEndpoint (for testing) ───

// NoopEndpoint always succeeds — useful for tests.
type NoopEndpoint struct {
	Delivered []feedback.Action
	mu        sync.Mutex
}

func (n *NoopEndpoint) Deliver(_ context.Context, action feedback.Action) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.Delivered = append(n.Delivered, action)
	return nil
}

// ─── FailingEndpoint (for testing) ───

// FailingEndpoint fails the first N deliveries, then succeeds.
type FailingEndpoint struct {
	FailCount int
	failed    int
	Delivered []feedback.Action
	mu        sync.Mutex
}

func (f *FailingEndpoint) Deliver(_ context.Context, action feedback.Action) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed++
	if f.failed <= f.FailCount {
		return fmt.Errorf("simulated failure %d/%d", f.failed, f.FailCount)
	}
	f.Delivered = append(f.Delivered, action)
	return nil
}
