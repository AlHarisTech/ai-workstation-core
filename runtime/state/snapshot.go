package state

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type SnapshotRecord struct {
	SnapshotID    string           `json:"snapshot_id"`
	GeneratedAt   time.Time        `json:"generated_at"`
	Consistency   string           `json:"consistency"`
	TraceCount    int              `json:"trace_count"`
	Traces        []TraceRecord    `json:"traces"`
	WorkerHealth  interface{}      `json:"worker_health,omitempty"`
	MetricsData   interface{}      `json:"metrics_data,omitempty"`
}

type ReplayResult struct {
	Traces       []TraceRecord `json:"traces"`
	TotalCount   int           `json:"total_count"`
	SuccessCount int           `json:"success_count"`
	DeniedCount  int           `json:"denied_count"`
	ErrorCount   int           `json:"error_count"`
}

func (ss *StateStore) GenerateSnapshot(workerHealth, metricsData interface{}) (*SnapshotRecord, error) {
	return ss.generateSnapshotInternal(workerHealth, metricsData, Weak)
}

func (ss *StateStore) generateSnapshotInternal(workerHealth, metricsData interface{}, mode ConsistencyMode) (*SnapshotRecord, error) {
	tracesDir := filepath.Join(ss.root, "traces")
	entries, err := os.ReadDir(tracesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &SnapshotRecord{SnapshotID: "snap_empty", GeneratedAt: time.Now()}, nil
		}
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	traces := make([]TraceRecord, 0, len(files))
	for _, name := range files {
		var tr TraceRecord
		path := filepath.Join(tracesDir, name)
		if err := ss.writer.ReadJSON(path, &tr); err == nil {
			traces = append(traces, tr)
		}
	}

	prefix := "snap_w_"
	if mode == Strong {
		prefix = "snap_s_"
	}

	snap := &SnapshotRecord{
		SnapshotID:   prefix + time.Now().UTC().Format("20060102T150405"),
		GeneratedAt:  time.Now(),
		Consistency:  string(mode),
		TraceCount:   len(traces),
		Traces:       traces,
		WorkerHealth: workerHealth,
		MetricsData:  metricsData,
	}

	snapPath := filepath.Join(ss.root, "snapshots", snap.SnapshotID+".json")
	return snap, ss.writer.WriteJSON(snapPath, snap)
}

func (ss *StateStore) ReplayHistory() (*ReplayResult, error) {
	tracesDir := filepath.Join(ss.root, "traces")
	entries, err := os.ReadDir(tracesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &ReplayResult{}, nil
		}
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	result := &ReplayResult{}
	for _, name := range files {
		var tr TraceRecord
		path := filepath.Join(tracesDir, name)
		if err := ss.writer.ReadJSON(path, &tr); err != nil {
			continue
		}
		result.Traces = append(result.Traces, tr)
		result.TotalCount++
		switch tr.Status {
		case string(types.StatusSuccess):
			result.SuccessCount++
		case string(types.StatusDenied):
			result.DeniedCount++
		case string(types.StatusError):
			result.ErrorCount++
		}
	}
	return result, nil
}

func (ss *StateStore) CompactTraces(olderThan time.Duration) (int, error) {
	tracesDir := filepath.Join(ss.root, "traces")
	entries, err := os.ReadDir(tracesDir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-olderThan)
	compacted := 0

	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(tracesDir, e.Name())
			os.Remove(path)
			compacted++
		}
	}

	return compacted, nil
}

func (ss *StateStore) LoadSnapshot(snapshotID string) (*SnapshotRecord, error) {
	var snap SnapshotRecord
	path := filepath.Join(ss.root, "snapshots", snapshotID+".json")
	if err := ss.writer.ReadJSON(path, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

func (ss *StateStore) ListSnapshots() ([]string, error) {
	snapDir := filepath.Join(ss.root, "snapshots")
	entries, err := os.ReadDir(snapDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snaps []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			snaps = append(snaps, strings.TrimSuffix(e.Name(), ".json"))
		}
	}
	sort.Strings(snaps)
	return snaps, nil
}

func (ss *StateStore) SnapshotToJSON(snap *SnapshotRecord) ([]byte, error) {
	return json.MarshalIndent(snap, "", "  ")
}
