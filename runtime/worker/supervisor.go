package worker

import (
	"sync"
	"sync/atomic"
	"time"
)

type WorkerState string

const (
	WorkerInit       WorkerState = "init"
	WorkerReady      WorkerState = "ready"
	WorkerRunning    WorkerState = "running"
	WorkerExecuting  WorkerState = "executing"
	WorkerWaiting    WorkerState = "waiting"
	WorkerRecovering WorkerState = "recovering"
	WorkerFailed     WorkerState = "failed"
	WorkerRestarting WorkerState = "restarting"
)

type WorkerHealth struct {
	WorkerID      string      `json:"worker_id"`
	State         WorkerState `json:"state"`
	Failures      atomic.Int64
	Restarts      atomic.Int64
	LastHeartbeat time.Time   `json:"last_heartbeat"`
	RequestsDone  atomic.Int64
	mu            sync.Mutex
}

func (wh *WorkerHealth) Transition(to WorkerState) bool {
	wh.mu.Lock()
	defer wh.mu.Unlock()

	valid := map[WorkerState][]WorkerState{
		WorkerInit:       {WorkerReady},
		WorkerReady:      {WorkerRunning},
		WorkerRunning:    {WorkerExecuting, WorkerWaiting, WorkerRecovering, WorkerFailed},
		WorkerExecuting:  {WorkerWaiting, WorkerRecovering, WorkerFailed},
		WorkerWaiting:    {WorkerReady},
		WorkerRecovering: {WorkerReady, WorkerFailed},
		WorkerFailed:     {WorkerRestarting},
		WorkerRestarting: {WorkerInit, WorkerReady},
	}

	for _, allowed := range valid[wh.State] {
		if to == allowed {
			wh.State = to
			wh.LastHeartbeat = time.Now()
			return true
		}
	}
	return false
}

func (wh *WorkerHealth) MarkAlive() {
	wh.mu.Lock()
	wh.LastHeartbeat = time.Now()
	wh.mu.Unlock()
}

func (wh *WorkerHealth) MarkDegraded(reason string) {
	wh.mu.Lock()
	if wh.State == WorkerExecuting || wh.State == WorkerRunning {
		wh.State = WorkerRecovering
	}
	wh.Failures.Add(1)
	wh.LastHeartbeat = time.Now()
	wh.mu.Unlock()
}

func (wh *WorkerHealth) MarkFailed(reason string) {
	wh.mu.Lock()
	wh.State = WorkerFailed
	wh.Failures.Add(1)
	wh.LastHeartbeat = time.Now()
	wh.mu.Unlock()
}

func (wh *WorkerHealth) IncrementRequests() {
	wh.RequestsDone.Add(1)
}

func (wh *WorkerHealth) Snapshot() map[string]interface{} {
	wh.mu.Lock()
	defer wh.mu.Unlock()
	return map[string]interface{}{
		"worker_id":      wh.WorkerID,
		"state":          string(wh.State),
		"failures":       wh.Failures.Load(),
		"restarts":       wh.Restarts.Load(),
		"last_heartbeat": wh.LastHeartbeat.Format(time.RFC3339),
		"requests_done":  wh.RequestsDone.Load(),
	}
}

type WorkerSupervisor struct {
	health    map[string]*WorkerHealth
	mu        sync.Mutex
	maxRestarts int
}

func NewWorkerSupervisor() *WorkerSupervisor {
	return &WorkerSupervisor{
		health:      make(map[string]*WorkerHealth),
		maxRestarts: 3,
	}
}

func (ws *WorkerSupervisor) Register(workerID string) *WorkerHealth {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	wh := &WorkerHealth{WorkerID: workerID, State: WorkerInit, LastHeartbeat: time.Now()}
	ws.health[workerID] = wh
	return wh
}

func (ws *WorkerSupervisor) Get(workerID string) *WorkerHealth {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	return ws.health[workerID]
}

func (ws *WorkerSupervisor) AllHealth() map[string]map[string]interface{} {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	result := make(map[string]map[string]interface{}, len(ws.health))
	for id, wh := range ws.health {
		result[id] = wh.Snapshot()
	}
	return result
}

func (ws *WorkerSupervisor) Restart(workerID string) bool {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	wh, ok := ws.health[workerID]
	if !ok {
		return false
	}
	if wh.Restarts.Load() >= int64(ws.maxRestarts) {
		return false
	}
	wh.Restarts.Add(1)
	wh.State = WorkerRestarting
	return true
}

func (ws *WorkerSupervisor) Utilization() float64 {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	running := 0
	for _, wh := range ws.health {
		switch wh.State {
		case WorkerRunning, WorkerExecuting:
			running++
		}
	}
	if len(ws.health) == 0 {
		return 0
	}
	return float64(running) / float64(len(ws.health))
}

func (ws *WorkerSupervisor) FailureSummary() map[string]int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	result := map[string]int{
		"total_failures": 0, "total_restarts": 0,
		"degraded_workers": 0, "failed_workers": 0,
		"alive_workers": 0, "recovering_workers": 0,
	}
	for _, wh := range ws.health {
		result["total_failures"] += int(wh.Failures.Load())
		result["total_restarts"] += int(wh.Restarts.Load())
		switch wh.State {
		case WorkerReady, WorkerRunning, WorkerExecuting:
			result["alive_workers"]++
		case WorkerRecovering:
			result["recovering_workers"]++
		case WorkerFailed:
			result["failed_workers"]++
		}
	}
	return result
}
