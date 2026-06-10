package release

import (
	"context"
	"time"

	mcpobs "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/observability"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type ReleaseOrchestrator struct {
	github    *GitHubBridge
	queue     *ReleaseQueue
	idempotent *IdempotencyStore
	notify    func(mcpobs.TraceEvent)
	owner     string
	repo      string
	persistPath string
}

func NewReleaseOrchestrator(token, repoOwner, repoName string, notify func(mcpobs.TraceEvent)) *ReleaseOrchestrator {
	persistPath := ".ai/state/release_queue.json"
	ro := &ReleaseOrchestrator{
		github:      NewGitHubBridge(token, repoOwner, repoName),
		queue:       NewReleaseQueue(notify, persistPath),
		idempotent:  NewIdempotencyStore(),
		notify:      notify,
		owner:       repoOwner,
		repo:        repoName,
		persistPath: persistPath,
	}
	// Recover idempotency store from disk
	store, err := LoadIdempotencyStore("")
	if err == nil && len(store) > 0 {
		for k := range store {
			ro.idempotent.Mark(k)
		}
	}
	return ro
}

func (ro *ReleaseOrchestrator) Queue() *ReleaseQueue {
	return ro.queue
}

func (ro *ReleaseOrchestrator) IdempotencyStore() *IdempotencyStore {
	return ro.idempotent
}

func (ro *ReleaseOrchestrator) Name() string { return "mcp-release-orchestrator" }

func (ro *ReleaseOrchestrator) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	start := time.Now()
	input := ro.parseInput(req)
	traceID := req.ID

	ro.emit(mcpobs.TraceEvent{Type: mcpobs.EventReleaseStart, TraceID: traceID, SessionID: req.SessionID, Timestamp: time.Now().UnixMilli()})

	if input.Version == "" || input.CommitSHA == "" {
		return ro.fail(req.ID, "RELEASE_INPUT_INVALID: version and commit_sha required", start)
	}

	// Phase 1: Create tag locally
	tag := input.Version
	tagSHA, err := ro.github.CreateTag(tag, input.CommitSHA)
	if err != nil {
		errMsg := "TAG_CREATION_FAILED: " + err.Error()
		return ro.fail(req.ID, errMsg, start)
	}

	ro.emit(mcpobs.TraceEvent{Type: mcpobs.EventReleaseTagCreated, TraceID: traceID, SessionID: req.SessionID, Detail: tag, Timestamp: time.Now().UnixMilli()})

	// Phase 2: Check external dependency availability
	if err := ro.github.ValidateAccess(); err != nil {
		// Transition to PENDING_EXTERNAL — not a failure
		ro.emit(mcpobs.TraceEvent{
			Type:    mcpobs.EventReleasePendingExternal,
			TraceID: traceID, SessionID: req.SessionID,
			Error: err.Error(), Detail: tag,
			Timestamp: time.Now().UnixMilli(),
		})

		entry := ro.queue.Enqueue(input, ro.owner, ro.repo)
		entry.Retry.Attempts = 1

		ro.emit(mcpobs.TraceEvent{
			Type:    mcpobs.EventReleaseRetryScheduled,
			TraceID: traceID,
			Detail:  entry.ReleaseID,
			Timestamp: time.Now().UnixMilli(),
		})

		return types.MCPResponse{
			ID:        req.ID,
			Success:   true,
			LatencyMS: time.Since(start).Milliseconds(),
			Data: map[string]any{
				"status":     "PENDING_EXTERNAL",
				"tag":        tag,
				"commit_sha": input.CommitSHA,
				"release_id": entry.ReleaseID,
				"reason":     err.Error(),
			},
		}, nil
	}

	// Phase 3: Create release on GitHub
	releaseURL, err := ro.github.CreateRelease(tag, input.CommitSHA, input.ReleaseNotes)
	if err != nil {
		ro.github.DeleteTag(tag, tagSHA)
		return ro.fail(req.ID, "RELEASE_CREATION_FAILED: "+err.Error(), start)
	}

	ro.emit(mcpobs.TraceEvent{Type: mcpobs.EventReleasePublished, TraceID: traceID, SessionID: req.SessionID, Detail: releaseURL, Timestamp: time.Now().UnixMilli()})

	return types.MCPResponse{
		ID:        req.ID,
		Success:   true,
		LatencyMS: time.Since(start).Milliseconds(),
		Data: map[string]any{
			"status":      "COMPLETED",
			"tag":         tag,
			"commit_sha":  input.CommitSHA,
			"release_url": releaseURL,
		},
	}, nil
}

func (ro *ReleaseOrchestrator) ProcessQueue() int {
	processed := 0
	for {
		entry, ok := ro.queue.Dequeue()
		if !ok {
			break
		}

		// Idempotency check: skip if already completed
		if ro.idempotent.Exists(entry.IdempotencyKey) {
			if url, ok := ro.github.ReleaseExists(entry.Version); ok {
				ro.emit(mcpobs.TraceEvent{
					Type:      mcpobs.EventReleaseExternalRecovered,
					Detail:    url,
					Timestamp: time.Now().UnixMilli(),
				})
				ro.queue.Complete(entry.ReleaseID, true, "")
				processed++
				continue
			}
		}

		if err := ro.github.ValidateAccess(); err != nil {
			ro.emit(mcpobs.TraceEvent{
				Type:      mcpobs.EventReleaseRetryScheduled,
				Detail:    entry.ReleaseID,
				Error:     err.Error(),
				Timestamp: time.Now().UnixMilli(),
			})
			ro.queue.Complete(entry.ReleaseID, false, err.Error())
			continue
		}

		// GitHub dedup: check if tag already exists
		if ro.github.TagExists(entry.Version) {
			// Tag exists — skip creation, try release
		} else {
			_, err := ro.github.CreateTag(entry.Version, entry.CommitSHA)
			if err != nil {
				ro.emit(mcpobs.TraceEvent{
					Type:      mcpobs.EventReleaseRetryScheduled,
					Detail:    entry.ReleaseID,
					Error:     err.Error(),
					Timestamp: time.Now().UnixMilli(),
				})
				ro.queue.Complete(entry.ReleaseID, false, err.Error())
				continue
			}
			ro.emit(mcpobs.TraceEvent{
				Type:      mcpobs.EventReleaseTagCreated,
				Detail:    entry.Version,
				Timestamp: time.Now().UnixMilli(),
			})
		}

		// Check if release already exists
		releaseURL, exists := ro.github.ReleaseExists(entry.Version)
		if !exists {
			var err error
			releaseURL, err = ro.github.CreateRelease(entry.Version, entry.CommitSHA, entry.ReleaseNotes)
			if err != nil {
				ro.emit(mcpobs.TraceEvent{
					Type:      mcpobs.EventReleaseRetryScheduled,
					Detail:    entry.ReleaseID,
					Error:     err.Error(),
					Timestamp: time.Now().UnixMilli(),
				})
				ro.queue.Complete(entry.ReleaseID, false, err.Error())
				continue
			}
		}

		ro.idempotent.Mark(entry.IdempotencyKey)
		_ = PersistCompletedEntry(entry.IdempotencyKey, "")

		ro.emit(mcpobs.TraceEvent{
			Type:      mcpobs.EventReleaseExternalRecovered,
			Detail:    releaseURL,
			Timestamp: time.Now().UnixMilli(),
		})
		ro.queue.Complete(entry.ReleaseID, true, "")
		processed++
	}
	return processed
}

func (ro *ReleaseOrchestrator) parseInput(req types.MCPRequest) ReleaseInput {
	return ReleaseInput{
		Repo:         stringOrMap(req.Payload, "repo"),
		Version:      stringOrMap(req.Payload, "version"),
		CommitSHA:    stringOrMap(req.Payload, "commit_sha"),
		ReleaseNotes: stringOrMap(req.Payload, "release_notes"),
	}
}

func (ro *ReleaseOrchestrator) fail(id, errMsg string, start time.Time) (types.MCPResponse, error) {
	ro.emit(mcpobs.TraceEvent{Type: mcpobs.EventReleaseFailure, TraceID: id, Error: errMsg, Timestamp: time.Now().UnixMilli()})
	return types.MCPResponse{ID: id, Success: false, Error: errMsg, LatencyMS: time.Since(start).Milliseconds()}, nil
}

func (ro *ReleaseOrchestrator) emit(ev mcpobs.TraceEvent) {
	if ro.notify != nil {
		ro.notify(ev)
	}
}

func stringOrMap(payload map[string]any, key string) string {
	v, _ := payload[key].(string)
	return v
}
