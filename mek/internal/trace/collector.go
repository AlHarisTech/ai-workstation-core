package trace

import (
	"sync"
	"time"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/pkg/types"
)

// Span captures the timing of a single node execution.
type Span struct {
	ScheduledAt  string `json:"scheduled_at,omitempty"`
	DispatchedAt string `json:"dispatched_at,omitempty"`
	StartedAt    string `json:"started_at,omitempty"`
	CompletedAt  string `json:"completed_at,omitempty"`
	CommittedAt  string `json:"committed_at,omitempty"`
	DurationMs   int    `json:"duration_ms"`
}

// NodeTrace is the complete record of a single execution unit.
type NodeTrace struct {
	TraceID     string                 `json:"trace_id"`
	NodeID      string                 `json:"node_id"`
	NodeType    types.UnitType         `json:"node_type"`
	Span        Span                   `json:"span"`
	Inputs      map[string]interface{} `json:"inputs,omitempty"`
	Outputs     map[string]interface{} `json:"outputs,omitempty"`
	Artifacts   []string               `json:"artifacts,omitempty"`
	TerminalStatus types.NodeStatus    `json:"terminal_status"`
	Sequence    uint64                 `json:"sequence"`
}

// Collector observes Commit Engine events and builds per-node execution traces.
// It is PASSIVE — it never affects execution ordering or determinism (OB-001).
type Collector struct {
	mu     sync.RWMutex
	traces map[string]*NodeTrace // nodeID → trace
	events []TraceEvent
	seq    uint64
}

// TraceEvent is a lightweight event emitted for external consumers.
type TraceEvent struct {
	Sequence uint64           `json:"sequence"`
	Type     string           `json:"type"` // "started", "committed", "failed", "terminated", "skipped"
	NodeID   string           `json:"node_id"`
	Status   types.NodeStatus `json:"status"`
	Trace    *NodeTrace       `json:"trace,omitempty"`
}

func NewCollector() *Collector {
	return &Collector{
		traces: make(map[string]*NodeTrace),
	}
}

// Subscribe consumes CommitEvents and builds execution traces.
// Must be called with the events channel from the Commit Engine.
func (c *Collector) Subscribe(events <-chan commit.CommitEvent) <-chan TraceEvent {
	out := make(chan TraceEvent, 1024)
	go func() {
		defer close(out)
		for evt := range events {
			c.process(evt, out)
		}
	}()
	return out
}

func (c *Collector) process(evt commit.CommitEvent, out chan<- TraceEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)

	trace, exists := c.traces[evt.NodeID]
	if !exists {
		trace = &NodeTrace{
			TraceID:  evt.NodeID + "-" + now,
			NodeID:   evt.NodeID,
			NodeType: inferType(evt.NodeID),
		}
		c.traces[evt.NodeID] = trace
	}

	// Track state transitions
	switch evt.NewStatus {
	case types.StatusRunning:
		trace.Span.StartedAt = now
		c.emit(out, "started", evt.NodeID, evt.NewStatus, trace)

	case types.StatusSuccess:
		trace.Span.CompletedAt = now
		trace.Span.CommittedAt = now
		trace.Outputs = evt.Outputs
		trace.Artifacts = evt.Artifacts
		trace.TerminalStatus = types.StatusSuccess
		c.emit(out, "committed", evt.NodeID, evt.NewStatus, trace)

	case types.StatusFailure:
		trace.Span.CompletedAt = now
		trace.Span.CommittedAt = now
		trace.Outputs = evt.Outputs
		trace.Artifacts = evt.Artifacts
		trace.TerminalStatus = types.StatusFailure
		c.emit(out, "failed", evt.NodeID, evt.NewStatus, trace)

	case types.StatusSkipped:
		trace.Span.CommittedAt = now
		trace.TerminalStatus = types.StatusSkipped
		c.emit(out, "skipped", evt.NodeID, evt.NewStatus, trace)

	case types.StatusTerminated:
		trace.Span.CommittedAt = now
		trace.TerminalStatus = types.StatusTerminated
		c.emit(out, "terminated", evt.NodeID, evt.NewStatus, trace)

	case types.StatusReady:
		trace.Span.ScheduledAt = now
		c.emit(out, "scheduled", evt.NodeID, evt.NewStatus, trace)
	}
}

func (c *Collector) emit(out chan<- TraceEvent, evtType, nodeID string, status types.NodeStatus, trace *NodeTrace) {
	c.seq++
	event := TraceEvent{
		Sequence: c.seq,
		Type:     evtType,
		NodeID:   nodeID,
		Status:   status,
		Trace:    traceSnapshot(trace),
	}
	select {
	case out <- event:
	default:
	}
}

// Trace returns the full trace for a node.
func (c *Collector) Trace(nodeID string) (*NodeTrace, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	t, ok := c.traces[nodeID]
	if ok {
		return traceSnapshot(t), true
	}
	return nil, false
}

// AllTraces returns all collected traces.
func (c *Collector) AllTraces() map[string]*NodeTrace {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]*NodeTrace, len(c.traces))
	for k, v := range c.traces {
		out[k] = traceSnapshot(v)
	}
	return out
}

// TerminalStatuses returns a map of nodeID → terminal status.
func (c *Collector) TerminalStatuses() map[string]types.NodeStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]types.NodeStatus, len(c.traces))
	for k, v := range c.traces {
		out[k] = v.TerminalStatus
	}
	return out
}

func traceSnapshot(t *NodeTrace) *NodeTrace {
	if t == nil {
		return nil
	}
	cp := *t
	if t.Outputs != nil {
		cp.Outputs = make(map[string]interface{}, len(t.Outputs))
		for k, v := range t.Outputs {
			cp.Outputs[k] = v
		}
	}
	if t.Artifacts != nil {
		cp.Artifacts = make([]string, len(t.Artifacts))
		copy(cp.Artifacts, t.Artifacts)
	}
	return &cp
}

func inferType(nodeID string) types.UnitType {
	// In production, this would come from CEG metadata.
	// For the reference implementation, we return a placeholder.
	_ = nodeID
	return types.UnitCapability
}
