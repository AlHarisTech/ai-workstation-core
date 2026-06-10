package release

import (
	"encoding/json"
	"sync"
	"time"

	mcpobs "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/observability"
)

type ReleaseStatus string

const (
	StatusPendingExternal ReleaseStatus = "PENDING_EXTERNAL"
	StatusQueued          ReleaseStatus = "QUEUED"
	StatusPublishing      ReleaseStatus = "PUBLISHING"
	StatusCompleted       ReleaseStatus = "COMPLETED"
	StatusFinalFailed     ReleaseStatus = "FINAL_FAILED"
)

type ReleaseEntry struct {
	ReleaseID      string        `json:"release_id"`
	CommitSHA      string        `json:"commit_sha"`
	Version        string        `json:"version"`
	RepoOwner      string        `json:"repo_owner"`
	RepoName       string        `json:"repo_name"`
	ReleaseNotes   string        `json:"release_notes"`
	Status         ReleaseStatus `json:"status"`
	Retry          RetryState    `json:"retry"`
	IdempotencyKey string        `json:"idempotency_key"`
	CreatedAt      int64         `json:"created_at"`
	UpdatedAt      int64         `json:"updated_at"`
	LastError      string        `json:"last_error,omitempty"`
}

type ReleaseQueue struct {
	mu          sync.Mutex
	entries     []ReleaseEntry
	notify      func(mcpobs.TraceEvent)
	persistPath string
}

func NewReleaseQueue(notify func(mcpobs.TraceEvent), persistPath string) *ReleaseQueue {
	q := &ReleaseQueue{
		entries:     make([]ReleaseEntry, 0),
		notify:      notify,
		persistPath: persistPath,
	}
	if persistPath != "" {
		_ = q.recover()
	}
	return q
}

func (q *ReleaseQueue) recover() error {
	entries, err := LoadQueue(q.persistPath)
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].Status == StatusPendingExternal || entries[i].Status == StatusQueued {
			entries[i].Status = StatusPendingExternal
		}
	}
	q.entries = entries
	return nil
}

func (q *ReleaseQueue) persist() {
	if q.persistPath == "" {
		return
	}
	_ = PersistQueue(q.entries, q.persistPath)
}

func (q *ReleaseQueue) Enqueue(input ReleaseInput, owner, repo string) ReleaseEntry {
	q.mu.Lock()
	defer q.mu.Unlock()

	key := IdempotencyKey(input.CommitSHA, input.Version)
	entry := ReleaseEntry{
		ReleaseID:      input.Version + "@" + truncateSHA(input.CommitSHA, 8),
		CommitSHA:      input.CommitSHA,
		Version:        input.Version,
		RepoOwner:      owner,
		RepoName:       repo,
		ReleaseNotes:   input.ReleaseNotes,
		Status:         StatusPendingExternal,
		Retry:          NewRetryState(input.CommitSHA, input.Version),
		IdempotencyKey: key,
		CreatedAt:      time.Now().UnixMilli(),
		UpdatedAt:      time.Now().UnixMilli(),
	}

	q.entries = append(q.entries, entry)
	q.persist()

	if q.notify != nil {
		q.notify(mcpobs.TraceEvent{
			Type:      mcpobs.EventReleaseQueued,
			Detail:    entry.ReleaseID,
			Status:    string(entry.Status),
			Timestamp: time.Now().UnixMilli(),
		})
	}

	return entry
}

func (q *ReleaseQueue) Dequeue() (ReleaseEntry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, e := range q.entries {
		if e.Status == StatusPendingExternal || (e.Status == StatusQueued && IsRetryDue(e.Retry)) {
			q.entries[i].Status = StatusPublishing
			q.entries[i].UpdatedAt = time.Now().UnixMilli()
			q.persist()
			return q.entries[i], true
		}
	}
	return ReleaseEntry{}, false
}

func (q *ReleaseQueue) Complete(releaseID string, success bool, errMsg string) {
	q.mu.Lock()
	defer q.mu.Unlock()

	for i, e := range q.entries {
		if e.ReleaseID == releaseID {
			if success {
				q.entries[i].Status = StatusCompleted
			} else {
				rs := AdvanceRetryState(e.Retry, e.CommitSHA, e.Version)
				q.entries[i].Retry = rs
				q.entries[i].LastError = errMsg
				if rs.Finalized {
					q.entries[i].Status = StatusFinalFailed
				} else {
					q.entries[i].Status = StatusQueued
				}
			}
			q.entries[i].UpdatedAt = time.Now().UnixMilli()
			q.persist()
			return
		}
	}
}

func (q *ReleaseQueue) PendingCount() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	count := 0
	for _, e := range q.entries {
		if e.Status == StatusPendingExternal || e.Status == StatusQueued || e.Status == StatusPublishing {
			count++
		}
	}
	return count
}

func (q *ReleaseQueue) Snapshot() []ReleaseEntry {
	q.mu.Lock()
	defer q.mu.Unlock()
	snap := make([]ReleaseEntry, len(q.entries))
	copy(snap, q.entries)
	return snap
}

func (q *ReleaseQueue) JSON() string {
	q.mu.Lock()
	defer q.mu.Unlock()
	raw, _ := json.MarshalIndent(q.entries, "", "  ")
	return string(raw)
}

func truncateSHA(sha string, n int) string {
	if len(sha) < n {
		return sha
	}
	return sha[:n]
}
