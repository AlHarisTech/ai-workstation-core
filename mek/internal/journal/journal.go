package journal

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/pkg/types"
)

// Entry represents a single journaled state transition.
// Immutable after creation. Append-only.
type Entry struct {
	Sequence  uint64           `json:"sequence"`
	Timestamp string           `json:"timestamp"`
	NodeID    string           `json:"node_id"`
	FromStatus types.NodeStatus `json:"from_status"`
	ToStatus  types.NodeStatus `json:"to_status"`
	Outputs   map[string]interface{} `json:"outputs,omitempty"`
	Artifacts []string          `json:"artifacts,omitempty"`
}

// Journal is a durable append-only record of execution state transitions.
// It observes the Commit Engine passively through event subscription.
// It adds NO latency to the execution path — writes are asynchronous.
type Journal struct {
	mu       sync.Mutex
	file     *os.File
	encoder  *json.Encoder
	entries  []Entry
	sequence uint64
}

// New creates a Journal that writes to the given file path.
func New(path string) (*Journal, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("journal: create file: %w", err)
	}
	return &Journal{
		file:    f,
		encoder: json.NewEncoder(f),
		entries: make([]Entry, 0),
	}, nil
}

// Subscribe returns a channel that receives CommitEvents and records them.
// The caller should run this in a goroutine. Events are written to the journal
// file and stored in memory for replay.
func (j *Journal) Subscribe(events <-chan commit.CommitEvent) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for evt := range events {
			j.record(evt)
		}
	}()
	return done
}

func (j *Journal) record(evt commit.CommitEvent) {
	j.mu.Lock()
	defer j.mu.Unlock()

	entry := Entry{
		Sequence:   j.sequence,
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
		NodeID:     evt.NodeID,
		ToStatus:   evt.NewStatus,
		Outputs:    evt.Outputs,
		Artifacts:  evt.Artifacts,
	}
	// Infer from_status from prior entry if available
	for i := len(j.entries) - 1; i >= 0; i-- {
		if j.entries[i].NodeID == evt.NodeID {
			entry.FromStatus = j.entries[i].ToStatus
			break
		}
	}

	j.entries = append(j.entries, entry)
	j.encoder.Encode(entry) // async-safe: single goroutine writes to file
	j.sequence++
}

// Entries returns all recorded journal entries (in-memory snapshot).
func (j *Journal) Entries() []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]Entry, len(j.entries))
	copy(out, j.entries)
	return out
}

// Sequence returns the current sequence number.
func (j *Journal) Sequence() uint64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.sequence
}

// Close flushes and closes the journal file.
func (j *Journal) Close() error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.file.Close()
}

// NodeHistory returns all entries for a specific node, ordered by sequence.
func (j *Journal) NodeHistory(nodeID string) []Entry {
	j.mu.Lock()
	defer j.mu.Unlock()
	var result []Entry
	for _, e := range j.entries {
		if e.NodeID == nodeID {
			result = append(result, e)
		}
	}
	return result
}

// TerminalStatus returns the final status of a node from the journal.
func (j *Journal) TerminalStatus(nodeID string) (types.NodeStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()
	for i := len(j.entries) - 1; i >= 0; i-- {
		if j.entries[i].NodeID == nodeID {
			return j.entries[i].ToStatus, true
		}
	}
	return "", false
}
