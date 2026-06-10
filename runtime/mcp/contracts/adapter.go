package contracts

import (
	"context"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type StandardAdapter struct {
	name    string
	handler func(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error)
}

func NewStandardAdapter(name string, handler func(context.Context, types.MCPRequest) (types.MCPResponse, error)) *StandardAdapter {
	return &StandardAdapter{name: name, handler: handler}
}

func (sa *StandardAdapter) Name() string { return sa.name }

func (sa *StandardAdapter) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	return sa.handler(ctx, req)
}

type RetryConfig struct {
	MaxRetries int
	BaseDelay  time.Duration
	MaxDelay   time.Duration
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 2,
		BaseDelay:  100 * time.Millisecond,
		MaxDelay:   1 * time.Second,
	}
}

type RetryAdapter struct {
	inner  types.MCPAdapter
	config RetryConfig
}

func NewRetryAdapter(inner types.MCPAdapter, cfg RetryConfig) *RetryAdapter {
	if cfg.MaxRetries <= 0 {
		cfg.MaxRetries = 2
	}
	return &RetryAdapter{inner: inner, config: cfg}
}

func (ra *RetryAdapter) Name() string { return ra.inner.Name() }

func (ra *RetryAdapter) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	budget := GetRetryBudget(ctx)
	var lastErr error
	for attempt := 0; attempt <= ra.config.MaxRetries; attempt++ {
		if attempt > 0 && budget != nil && !budget.TryConsume(req.Tool) {
			return types.MCPResponse{
				ID: req.ID, Success: false,
				Error: "retry budget exhausted for " + req.Tool,
			}, lastErr
		}
		resp, err := ra.inner.Execute(ctx, req)
		if err == nil && resp.Success {
			return resp, nil
		}
		lastErr = err
		if attempt < ra.config.MaxRetries {
			delay := jitteredDelay(ra.config.BaseDelay, attempt, ra.config.MaxDelay)
			time.Sleep(delay)
		}
	}
	return types.MCPResponse{
		ID: req.ID, Success: false,
		Error: "retry exhausted: " + lastErr.Error(),
	}, lastErr
}

type TimeoutAdapter struct {
	inner   types.MCPAdapter
	timeout time.Duration
}

func NewTimeoutAdapter(inner types.MCPAdapter, timeout time.Duration) *TimeoutAdapter {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &TimeoutAdapter{inner: inner, timeout: timeout}
}

func (ta *TimeoutAdapter) Name() string { return ta.inner.Name() }

func (ta *TimeoutAdapter) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, ta.timeout)
	defer cancel()

	type result struct {
		resp types.MCPResponse
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		r, e := ta.inner.Execute(ctx, req)
		ch <- result{r, e}
	}()

	select {
	case res := <-ch:
		return res.resp, res.err
	case <-ctx.Done():
		return types.MCPResponse{
			ID: req.ID, Success: false,
			Error: "TIMEOUT: " + ta.timeout.String(),
		}, ctx.Err()
	}
}

type FallbackAdapter struct {
	inner    types.MCPAdapter
	fallback types.MCPAdapter
}

func NewFallbackAdapter(inner, fallback types.MCPAdapter) *FallbackAdapter {
	return &FallbackAdapter{inner: inner, fallback: fallback}
}

func (fa *FallbackAdapter) Name() string { return fa.inner.Name() }

func (fa *FallbackAdapter) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	resp, err := fa.inner.Execute(ctx, req)
	if err != nil || !resp.Success {
		return fa.fallback.Execute(ctx, req)
	}
	return resp, nil
}
