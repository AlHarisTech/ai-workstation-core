package dispatcher

import (
	"fmt"
	"time"

	"github.com/anomalyco/mek/pkg/types"
)

type AdapterConfig struct {
	AgentAdapters map[string]AgentAdapterConfig `json:"agent_adapters" yaml:"agent_adapters"`
	ToolAdapters  map[string]ToolAdapterConfig  `json:"tool_adapters" yaml:"tool_adapters"`
	Providers     map[string]ProviderConfig     `json:"providers" yaml:"providers"`
}

type AgentAdapterConfig struct {
	Type              string   `json:"type" yaml:"type"`
	Provider          string   `json:"provider" yaml:"provider"`
	Allowlist         []string `json:"allowlist" yaml:"allowlist"`
	MaxToolInvocations int     `json:"max_tool_invocations" yaml:"max_tool_invocations"`
	MaxDurationMs     int      `json:"max_duration_ms" yaml:"max_duration_ms"`
	Isolation         string   `json:"isolation" yaml:"isolation"`
	FilesystemAccess  string   `json:"filesystem_access" yaml:"filesystem_access"`
	NetworkAccess     string   `json:"network_access" yaml:"network_access"`
}

type ToolAdapterConfig struct {
	Provider         string `json:"provider" yaml:"provider"`
	Idempotent       bool   `json:"idempotent" yaml:"idempotent"`
	TimeoutMs        int    `json:"timeout_ms" yaml:"timeout_ms"`
	Isolation        string `json:"isolation" yaml:"isolation"`
	FilesystemAccess string `json:"filesystem_access" yaml:"filesystem_access"`
	NetworkAccess    string `json:"network_access" yaml:"network_access"`
}

type ProviderConfig struct {
	Type   string                 `json:"type" yaml:"type"`
	Config map[string]interface{} `json:"config" yaml:"config"`
}

type Adapter interface {
	Execute(inputs map[string]interface{}, ctx *types.ExecutionContext) (*types.ExecutionResult, error)
}

type Dispatcher struct {
	ceg          *types.CEG
	adapterCfg   *AdapterConfig
	adapters     map[string]Adapter
}

func New(ceg *types.CEG, cfg *AdapterConfig) *Dispatcher {
	return &Dispatcher{
		ceg:        ceg,
		adapterCfg: cfg,
		adapters:   make(map[string]Adapter),
	}
}

func (d *Dispatcher) Register(name string, adapter Adapter) {
	d.adapters[name] = adapter
}

func (d *Dispatcher) Dispatch(nodeID string) (*types.ExecutionResult, error) {
	node := d.ceg.NodeMap[nodeID]
	if node == nil {
		return nil, fmt.Errorf("node %s not found in CEG", nodeID)
	}

	// Gate nodes: native execution — no adapter, no sandbox, no provider
	if node.Type == types.UnitGate {
		return d.dispatchGate(node)
	}

	// All other node types: resolve adapter and execute
	return d.dispatchAdapter(node)
}

func (d *Dispatcher) dispatchGate(node *types.CEGNode) (*types.ExecutionResult, error) {
	if node.Gate == nil {
		return nil, fmt.Errorf("gate node %s has no gate configuration", node.ID)
	}

	branch, err := d.evaluatePredicate(node.Gate)
	if err != nil {
		return nil, fmt.Errorf("gate %s predicate evaluation: %w", node.ID, err)
	}

	// ESCALATE is a reserved gate branch target (ADR-0003-A §6).
	// Valid only in Mode 2 — which is MEK v1's sole supported mode.
	// MEK treats ESCALATE as an implicit TERMINATE signal.
	if branch.Target == "ESCALATE" {
		return &types.ExecutionResult{
			Status:     types.StatusTerminated,
			Escalation: true,
			Metrics: types.ExecutionMetrics{
				DurationMs: 0,
			},
		}, nil
	}

	return &types.ExecutionResult{
		Status:         types.StatusSuccess,
		SelectedBranch: branch.Target,
		Metrics: types.ExecutionMetrics{
			DurationMs: 0,
		},
	}, nil
}

func (d *Dispatcher) evaluatePredicate(gate *types.GateConfig) (*types.GateBranch, error) {
	// In MEK v1, predicates are simplified: the first branch whose condition
	// is non-empty and does not start with "conditional:" is selected.
	// This is a deliberate simplification for the reference implementation.
	// A full predicate engine would evaluate declarative expressions.
	for i := range gate.Branches {
		b := &gate.Branches[i]
		if b.Condition != "" && len(b.Condition) > len("conditional:") &&
			b.Condition[:12] != "conditional:" {
			return b, nil
		}
	}
	// Default branch
	return &types.GateBranch{Target: gate.Default, Condition: "default"}, nil
}

func (d *Dispatcher) dispatchAdapter(node *types.CEGNode) (*types.ExecutionResult, error) {
	start := time.Now()

	// Try agent adapter first
	if node.Type == types.UnitAgent {
		if cfg, ok := d.adapterCfg.AgentAdapters[node.Binding.Contract]; ok {
			result, err := d.executeWithTimeout(node, &cfg, start)
			if err != nil {
				return nil, err
			}
			return result, nil
		}
	}

	// Try tool adapter
	if cfg, ok := d.adapterCfg.ToolAdapters[node.Binding.Contract]; ok {
		result, err := d.executeWithTimeout(node, &cfg, start)
		if err != nil {
			return nil, err
		}
		return result, nil
	}

	// Fallback: use registered adapter
	if adapter, ok := d.adapters[node.Binding.Contract]; ok {
		ctx := d.buildContext(node)
		result, err := adapter.Execute(nil, ctx)
		duration := int(time.Since(start).Milliseconds())
		if err != nil {
			return &types.ExecutionResult{
				Status: types.StatusFailure,
				Outputs: map[string]interface{}{
					"error": err.Error(),
				},
				Metrics: types.ExecutionMetrics{DurationMs: duration},
			}, nil
		}
		result.Metrics.DurationMs = duration
		return result, nil
	}

	// Built-in adapters for testing
	if node.Binding.Contract == "internal/noop" {
		duration := int(time.Since(start).Milliseconds())
		return &types.ExecutionResult{
			Status:  types.StatusSuccess,
			Metrics: types.ExecutionMetrics{DurationMs: duration},
		}, nil
	}
	if node.Binding.Contract == "internal/echo" {
		duration := int(time.Since(start).Milliseconds())
		return &types.ExecutionResult{
			Status:  types.StatusSuccess,
			Outputs: map[string]interface{}{"echo": node.ID},
			Metrics: types.ExecutionMetrics{DurationMs: duration},
		}, nil
	}

	return nil, fmt.Errorf("no adapter found for contract %s (node %s type %s)",
		node.Binding.Contract, node.ID, node.Type)
}

func (d *Dispatcher) executeWithTimeout(node *types.CEGNode, cfg interface{}, start time.Time) (*types.ExecutionResult, error) {
	var timeoutMs int
	switch c := cfg.(type) {
	case *AgentAdapterConfig:
		timeoutMs = c.MaxDurationMs
	case *ToolAdapterConfig:
		timeoutMs = c.TimeoutMs
	}

	ctx := d.buildContext(node)
	_ = ctx // context propagation placeholder

	// In the reference implementation, no real provider is invoked.
	// This simulates a successful execution for testing purposes.
	duration := int(time.Since(start).Milliseconds())
	_ = timeoutMs // timeout enforcement placeholder

	return &types.ExecutionResult{
		Status: types.StatusSuccess,
		Outputs: map[string]interface{}{
			"message": fmt.Sprintf("node %s executed successfully", node.ID),
		},
		Metrics: types.ExecutionMetrics{DurationMs: duration},
	}, nil
}

func (d *Dispatcher) buildContext(node *types.CEGNode) *types.ExecutionContext {
	ctx := &types.ExecutionContext{
		NodeID: node.ID,
	}

	// Determine isolation from adapter config
	if cfg, ok := d.adapterCfg.ToolAdapters[node.Binding.Contract]; ok {
		ctx.FilesystemScope = cfg.FilesystemAccess
		ctx.NetworkScope = cfg.NetworkAccess
		ctx.TimeoutMs = cfg.TimeoutMs
	}
	if cfg, ok := d.adapterCfg.AgentAdapters[node.Binding.Contract]; ok {
		ctx.FilesystemScope = cfg.FilesystemAccess
		ctx.NetworkScope = cfg.NetworkAccess
		ctx.TimeoutMs = cfg.MaxDurationMs
	}

	return ctx
}
