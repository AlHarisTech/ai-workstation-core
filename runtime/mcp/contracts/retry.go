package contracts

import (
	"context"
	"math/rand"
	"sync"
	"time"
)

type retryBudgetKey struct{}

type RetryBudgetConfig struct {
	TotalBudget int
	PerToolCap  int
}

func DefaultRetryBudgetConfig() RetryBudgetConfig {
	return RetryBudgetConfig{
		TotalBudget: 3,
		PerToolCap:  2,
	}
}

type RetryBudget struct {
	mu         sync.Mutex
	totalCap   int
	totalUsed  int
	perToolCap int
	toolUsed   map[string]int
}

func NewRetryBudget(cfg RetryBudgetConfig) *RetryBudget {
	if cfg.TotalBudget <= 0 {
		cfg.TotalBudget = 3
	}
	if cfg.PerToolCap <= 0 {
		cfg.PerToolCap = 2
	}
	return &RetryBudget{
		totalCap:   cfg.TotalBudget,
		perToolCap: cfg.PerToolCap,
		toolUsed:   make(map[string]int),
	}
}

func (rb *RetryBudget) TryConsume(tool string) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	if rb.totalUsed >= rb.totalCap {
		return false
	}
	if rb.toolUsed[tool] >= rb.perToolCap {
		return false
	}
	rb.totalUsed++
	rb.toolUsed[tool]++
	return true
}

func (rb *RetryBudget) Remaining() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.totalCap - rb.totalUsed
}

func WithRetryBudget(ctx context.Context, budget *RetryBudget) context.Context {
	return context.WithValue(ctx, retryBudgetKey{}, budget)
}

func GetRetryBudget(ctx context.Context) *RetryBudget {
	b, _ := ctx.Value(retryBudgetKey{}).(*RetryBudget)
	return b
}

func jitteredDelay(base time.Duration, attempt int, max time.Duration) time.Duration {
	d := base * time.Duration(1<<attempt)
	if d > max {
		d = max
	}
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d + jitter
}

func clampDelay(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}
