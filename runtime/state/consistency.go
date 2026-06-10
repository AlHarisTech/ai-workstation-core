package state

import "sync"

type ConsistencyMode string

const (
	Strong ConsistencyMode = "strong"
	Weak   ConsistencyMode = "weak"
)

type ConsistencyGuard struct {
	mu       sync.RWMutex
	mode     ConsistencyMode
	writing  bool
}

func NewConsistencyGuard() *ConsistencyGuard {
	return &ConsistencyGuard{mode: Weak}
}

func (cg *ConsistencyGuard) SetMode(mode ConsistencyMode) {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.mode = mode
}

func (cg *ConsistencyGuard) Mode() ConsistencyMode {
	cg.mu.RLock()
	defer cg.mu.RUnlock()
	return cg.mode
}

func (cg *ConsistencyGuard) BeginStrongWrite() bool {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	if cg.writing {
		return false
	}
	cg.writing = true
	return true
}

func (cg *ConsistencyGuard) EndStrongWrite() {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	cg.writing = false
}

func (cg *ConsistencyGuard) TakeStrongSnapshot(store *StateStore, workerHealth, metricsData interface{}) (*SnapshotRecord, error) {
	if !cg.BeginStrongWrite() {
		return nil, nil
	}
	defer cg.EndStrongWrite()
	return store.generateSnapshotInternal(workerHealth, metricsData, Strong)
}

func (cg *ConsistencyGuard) TakeWeakSnapshot(store *StateStore, metricsData interface{}) (*SnapshotRecord, error) {
	return store.generateSnapshotInternal(nil, metricsData, Weak)
}
