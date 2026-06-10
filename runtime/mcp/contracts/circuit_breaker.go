package contracts

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type CircuitState int

const (
	StateClosed   CircuitState = iota
	StateHalfOpen
	StateOpen
)

func (cs CircuitState) String() string {
	switch cs {
	case StateClosed:
		return "CLOSED"
	case StateHalfOpen:
		return "HALF_OPEN"
	case StateOpen:
		return "OPEN"
	default:
		return "UNKNOWN"
	}
}

type CircuitBreakerConfig struct {
	FailureThreshold    int
	LatencyThreshold    time.Duration
	RecoveryTimeout     time.Duration
	HalfOpenMaxRequests int
}

func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold:    5,
		LatencyThreshold:    5 * time.Second,
		RecoveryTimeout:     30 * time.Second,
		HalfOpenMaxRequests: 3,
	}
}

type CircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	consecutiveFail int
	lastFailureTime time.Time
	halfOpenSent    int
	config          CircuitBreakerConfig
	inner           types.MCPAdapter
	latencyThreshold time.Duration
}

func NewCircuitBreaker(inner types.MCPAdapter, config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 5
	}
	if config.LatencyThreshold <= 0 {
		config.LatencyThreshold = 5 * time.Second
	}
	if config.RecoveryTimeout <= 0 {
		config.RecoveryTimeout = 30 * time.Second
	}
	if config.HalfOpenMaxRequests <= 0 {
		config.HalfOpenMaxRequests = 3
	}
	return &CircuitBreaker{
		state:           StateClosed,
		config:          config,
		inner:           inner,
		latencyThreshold: config.LatencyThreshold,
	}
}

func (cb *CircuitBreaker) Name() string {
	return "circuit_breaker:" + cb.inner.Name()
}

func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

func (cb *CircuitBreaker) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	cb.mu.Lock()
	state := cb.state

	if state == StateOpen {
		if time.Since(cb.lastFailureTime) > cb.config.RecoveryTimeout {
			cb.state = StateHalfOpen
			cb.halfOpenSent = 0
			state = StateHalfOpen
		} else {
			cb.mu.Unlock()
			return types.MCPResponse{
				ID:      req.ID,
				Success: false,
				Error:   fmt.Sprintf("CIRCUIT_OPEN: %s is degraded, rejecting", cb.inner.Name()),
			}, fmt.Errorf("circuit breaker open for %s", cb.inner.Name())
		}
	}

	if state == StateHalfOpen && cb.halfOpenSent >= cb.config.HalfOpenMaxRequests {
		cb.mu.Unlock()
		return types.MCPResponse{
			ID:      req.ID,
			Success: false,
			Error:   fmt.Sprintf("CIRCUIT_HALF_OPEN: %s probe limit reached", cb.inner.Name()),
		}, fmt.Errorf("circuit breaker half-open probe limit for %s", cb.inner.Name())
	}

	if state == StateHalfOpen {
		cb.halfOpenSent++
	}
	cb.mu.Unlock()

	start := time.Now()
	resp, err := cb.inner.Execute(ctx, req)
	elapsed := time.Since(start)

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil || !resp.Success {
		cb.consecutiveFail++
		cb.lastFailureTime = time.Now()

		if cb.state == StateHalfOpen {
			cb.state = StateOpen
			cb.halfOpenSent = 0
		} else if cb.consecutiveFail >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}
		return resp, err
	}

	if elapsed >= cb.latencyThreshold {
		cb.consecutiveFail++
		cb.lastFailureTime = time.Now()
		if cb.consecutiveFail >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}
		return resp, err
	}

	if cb.state == StateHalfOpen {
		cb.state = StateClosed
	}
	cb.consecutiveFail = 0
	return resp, err
}

var _ = rand.Int
