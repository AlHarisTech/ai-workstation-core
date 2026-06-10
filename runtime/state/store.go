package state

import (
	"os"
	"path/filepath"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type StateStore struct {
	root    string
	writer  *AtomicWriter
}

type TraceRecord struct {
	RequestID        string                 `json:"request_id"`
	Status           string                 `json:"status"`
	ExecutionTrace   []types.StageResult    `json:"execution_trace"`
	DecisionPath     []string               `json:"decision_path"`
	StageTimings     map[string]float64     `json:"stage_timings"`
	WorkerID         string                 `json:"worker_id"`
	PipelineMode     string                 `json:"pipeline_mode"`
	PolicyGraph      []types.PolicyDecision `json:"policy_graph"`
	LatencyBreakdown types.LatencyBreakdown `json:"latency_breakdown"`
	Timestamp        time.Time              `json:"timestamp"`
}

type MetaRecord struct {
	Version         string `json:"version"`
	RegistryVersion string `json:"registry_version"`
	PolicyVersion   string `json:"policy_version"`
	Workers         int    `json:"workers"`
	QueueSize       int    `json:"queue_size"`
	UpdatedAt       string `json:"updated_at"`
}

func NewStateStore(root string) *StateStore {
	ss := &StateStore{
		root:   root,
		writer: NewAtomicWriter(),
	}
	os.MkdirAll(filepath.Join(root, "snapshots"), 0755)
	return ss
}

func (ss *StateStore) SaveTrace(ctx *types.RequestContext) error {
	return ss.saveTraceInternal(ctx, false)
}

func (ss *StateStore) SaveTraceWithRetry(ctx *types.RequestContext) error {
	if err := ss.saveTraceInternal(ctx, false); err != nil {
		_ = ss.saveTraceInternal(ctx, true)
		return err
	}
	return nil
}

func (ss *StateStore) saveTraceInternal(ctx *types.RequestContext, retry bool) error {
	record := TraceRecord{
		RequestID:        ctx.RequestID,
		Status:           string(ctx.Status),
		ExecutionTrace:   ctx.ExecutionTrace,
		DecisionPath:     ctx.DecisionPath,
		StageTimings:     ctx.StageTimings,
		WorkerID:         ctx.WorkerID,
		PipelineMode:     string(ctx.PipelineMode),
		PolicyGraph:      ctx.PolicyGraph,
		LatencyBreakdown: ctx.LatencyBreakdown,
		Timestamp:        ctx.TimestampEnd,
	}
	path := filepath.Join(ss.root, "traces", ctx.RequestID+".json")
	return ss.writer.WriteJSON(path, record)
}

func (ss *StateStore) SaveSession(sessionID, projectID, userID string) error {
	session := map[string]interface{}{
		"session_id": sessionID,
		"project_id": projectID,
		"user_id":    userID,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	}
	path := filepath.Join(ss.root, "sessions", sessionID+".json")
	return ss.writer.WriteJSON(path, session)
}

func (ss *StateStore) SaveMeta(meta MetaRecord) error {
	meta.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	path := filepath.Join(ss.root, "meta.json")
	return ss.writer.WriteJSON(path, meta)
}

func (ss *StateStore) LoadTrace(requestID string) (*TraceRecord, error) {
	var record TraceRecord
	path := filepath.Join(ss.root, "traces", requestID+".json")
	if err := ss.writer.ReadJSON(path, &record); err != nil {
		return nil, err
	}
	return &record, nil
}
