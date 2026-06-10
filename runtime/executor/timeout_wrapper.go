package executor

import (
	"context"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type TimeoutWrapper struct {
	core    *ExecutionCore
	timeout time.Duration
}

func NewTimeoutWrapper(core *ExecutionCore, timeout time.Duration) *TimeoutWrapper {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &TimeoutWrapper{core: core, timeout: timeout}
}

func (tw *TimeoutWrapper) ExecuteWithTimeout(ctx context.Context, toolID string, args map[string]interface{}) types.ExecResult {
	execCtx, cancel := context.WithTimeout(ctx, tw.timeout)
	defer cancel()
	return tw.core.Execute(execCtx, toolID, args)
}

func (tw *TimeoutWrapper) ExecuteIsolated(ctx context.Context, toolID string, args map[string]interface{}) types.ExecResult {
	start := time.Now()

	resultCh := make(chan types.ExecResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- types.ExecResult{
					Status:     "error",
					Error:      "worker panic recovered",
					ErrorCode:  "EXECUTION_PANIC",
					DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
				}
			}
		}()
		resultCh <- tw.ExecuteWithTimeout(ctx, toolID, args)
	}()

	select {
	case result := <-resultCh:
		return result
	case <-ctx.Done():
		return types.ExecResult{
			Status:     "error",
			Error:      "execution cancelled",
			ErrorCode:  "EXECUTION_CANCELLED",
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}
	}
}
