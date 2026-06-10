package executor

import (
	"context"
	"fmt"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type ExecutionCore struct {
	registry       ToolRegistry
	defaultTimeout time.Duration
}

type ToolRegistry interface {
	GetTool(toolID string) (*types.ToolDef, error)
}

func NewExecutionCore(registry ToolRegistry) *ExecutionCore {
	return &ExecutionCore{
		registry:       registry,
		defaultTimeout: 30 * time.Second,
	}
}

func (ec *ExecutionCore) Execute(ctx context.Context, toolID string, args map[string]interface{}) types.ExecResult {
	start := time.Now()

	toolDef, err := ec.registry.GetTool(toolID)
	if err != nil {
		return types.ExecResult{
			Status:     "error",
			Error:      err.Error(),
			ErrorCode:  "TOOL_NOT_FOUND",
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}
	}

	timeout := ec.defaultTimeout
	if toolDef.Governance.TimeoutMs > 0 {
		timeout = time.Duration(toolDef.Governance.TimeoutMs) * time.Millisecond
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	resultCh := make(chan types.ExecResult, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				resultCh <- types.ExecResult{
					Status:     "error",
					Error:      fmt.Sprint(r),
					ErrorCode:  "EXECUTION_ERROR",
					DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
				}
			}
		}()
		resultCh <- ec.dispatch(execCtx, toolDef, args)
	}()

	select {
	case result := <-resultCh:
		return result
	case <-execCtx.Done():
		return types.ExecResult{
			Status:     "error",
			Error:      "tool execution timed out",
			ErrorCode:  "EXECUTION_TIMEOUT",
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}
	}
}

func (ec *ExecutionCore) dispatch(ctx context.Context, toolDef *types.ToolDef, args map[string]interface{}) types.ExecResult {
	start := time.Now()

	switch toolDef.ID {
	case "echo":
		return types.ExecResult{
			Status:     "success",
			Result:     args,
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}
	default:
		return types.ExecResult{
			Status:     "error",
			Error:      "tool handler not implemented: " + toolDef.ID,
			ErrorCode:  "HANDLER_NOT_FOUND",
			DurationMs: float64(time.Since(start).Microseconds()) / 1000.0,
		}
	}
}
