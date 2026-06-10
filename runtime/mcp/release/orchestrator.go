package release

import (
	"context"
	"time"

	mcpobs "github.com/AlHarisTech/ai-workstation-core/runtime/mcp/observability"
	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type ReleaseOrchestrator struct {
	github *GitHubBridge
	queue  *ReleaseQueue
	notify func(mcpobs.TraceEvent)
	owner  string
	repo   string
}

func NewReleaseOrchestrator(token, repoOwner, repoName string, notify func(mcpobs.TraceEvent)) *ReleaseOrchestrator {
	return &ReleaseOrchestrator{
		github: NewGitHubBridge(token, repoOwner, repoName),
		queue:  NewReleaseQueue(notify),
		notify: notify,
		owner:  repoOwner,
		repo:   repoName,
	}
}

func (ro *ReleaseOrchestrator) Queue() *ReleaseQueue {
	return ro.queue
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

		input := ReleaseInput{
			Version:      entry.Version,
			CommitSHA:    entry.CommitSHA,
			ReleaseNotes: entry.ReleaseNotes,
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

		releaseURL, err := ro.github.CreateRelease(input.Version, input.CommitSHA, input.ReleaseNotes)
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
