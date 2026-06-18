package commit

import (
	"fmt"
	"sync"

	"github.com/anomalyco/mek/pkg/types"
)

type Engine struct {
	mu     sync.Mutex
	sm     *types.StatusMap
	events chan CommitEvent
}

type CommitEvent struct {
	NodeID    string
	NewStatus types.NodeStatus
	Outputs   map[string]interface{}
	Artifacts []string
}

func New(sm *types.StatusMap) *Engine {
	return &Engine{
		sm:     sm,
		events: make(chan CommitEvent, 1024),
	}
}

func (e *Engine) Events() <-chan CommitEvent {
	return e.events
}

// Commit atomically transitions a node to a new status.
// The Commit Engine is the SOLE WRITER of all node state (M-001).
func (e *Engine) Commit(nodeID string, newStatus types.NodeStatus, result *types.ExecutionResult) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	existing := e.sm.GetState(nodeID)
	if existing == nil {
		// First write — initialize
		e.sm.Init(nodeID, newStatus)
		if result != nil {
			s := e.sm.GetState(nodeID)
			if s != nil {
				s.Outputs = result.Outputs
				s.Artifacts = result.Artifacts
			}
		}
		e.publish(nodeID, newStatus, result)
		return nil
	}

	// M-003: Validate transition monotonicity
	if existing.Status.IsTerminal() {
		// DOUBLE_COMMIT_GUARD: already terminal — idempotent reject
		if existing.Status == newStatus {
			return nil // same status, no-op
		}
		return fmt.Errorf("DOUBLE_COMMIT: node %s already terminal (%s), cannot transition to %s",
			nodeID, existing.Status, newStatus)
	}

	if !isValidTransition(existing.Status, newStatus) {
		return fmt.Errorf("INVALID_TRANSITION: node %s cannot transition from %s to %s",
			nodeID, existing.Status, newStatus)
	}

	existing.Status = newStatus
	if result != nil {
		if result.Outputs != nil {
			existing.Outputs = result.Outputs
		}
		if result.Artifacts != nil {
			existing.Artifacts = result.Artifacts
		}
	}

	e.publish(nodeID, newStatus, result)
	return nil
}

func (e *Engine) publish(nodeID string, status types.NodeStatus, result *types.ExecutionResult) {
	event := CommitEvent{
		NodeID:    nodeID,
		NewStatus: status,
	}
	if result != nil {
		event.Outputs = result.Outputs
		event.Artifacts = result.Artifacts
	}
	select {
	case e.events <- event:
	default:
		// non-blocking — events are best-effort for observability
	}
}

func isValidTransition(from, to types.NodeStatus) bool {
	// See MEK §2.4 State Transitions
	valid := map[types.NodeStatus][]types.NodeStatus{
		types.StatusBlocked:    {types.StatusReady, types.StatusSkipped, types.StatusTerminated},
		types.StatusReady:      {types.StatusRunning, types.StatusTerminated},
		types.StatusRunning:    {types.StatusSuccess, types.StatusFailure, types.StatusTerminated},
		types.StatusSuccess:    {}, // terminal
		types.StatusFailure:    {}, // terminal
		types.StatusSkipped:    {}, // terminal
		types.StatusTerminated: {}, // terminal
	}
	for _, allowed := range valid[from] {
		if to == allowed {
			return true
		}
	}
	return false
}
