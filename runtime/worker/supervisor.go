package worker

import (
	"sync"
	"time"
)

type WorkerState string

const (
	WorkerAlive    WorkerState = "alive"
	WorkerDegraded WorkerState = "degraded"
	WorkerFailed   WorkerState = "failed"
)

type WorkerHealth struct {
	WorkerID      string      `json:"worker_id"`
	State         WorkerState `json:"state"`
	Failures      int         `json:"failures"`
	Restarts      int         `json:"restarts"`
	LastHeartbeat time.Time   `json:"last_heartbeat"`
	RequestsDone  int64       `json:"requests_done"`
	mu            sync.Mutex
}

func (wh *WorkerHealth) MarkAlive() {
	wh.mu.Lock()
	wh.State = WorkerAlive
	wh.LastHeartbeat = time.Now()
	wh.mu.Unlock()
}

func (wh *WorkerHealth) MarkDegraded(reason string) {
	wh.mu.Lock()
	wh.State = WorkerDegraded
	wh.Failures++
	wh.LastHeartbeat = time.Now()
	wh.mu.Unlock()
}

func (wh *WorkerHealth) MarkFailed(reason string) {
	wh.mu.Lock()
	wh.State = WorkerFailed
	wh.Failures++
	wh.LastHeartbeat = time.Now()
	wh.mu.Unlock()
}

func (wh *WorkerHealth) IncrementRequests() {
	wh.mu.Lock()
	wh.RequestsDone++
	wh.mu.Unlock()
}

func (wh *WorkerHealth) Snapshot() map[string]interface{} {
	wh.mu.Lock()
	defer wh.mu.Unlock()
	return map[string]interface{}{
		"worker_id":       wh.WorkerID,
		"state":           string(wh.State),
		"failures":        wh.Failures,
		"restarts":        wh.Restarts,
		"last_heartbeat":  wh.LastHeartbeat.Format(time.RFC3339),
		"requests_done":   wh.RequestsDone,
	}
}

type WorkerSupervisor struct {
	health  map[string]*WorkerHealth
	mu      sync.Mutex
	restartCb func(workerID string) bool
}

func NewWorkerSupervisor() *WorkerSupervisor {
	return &WorkerSupervisor{
		health: make(map[string]*WorkerHealth),
	}
}

func (ws *WorkerSupervisor) Register(workerID string) *WorkerHealth {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	wh := &WorkerHealth{WorkerID: workerID, State: WorkerAlive, LastHeartbeat: time.Now()}
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
	wh, ok := ws.health[workerID]
	if !ok {
		ws.mu.Unlock()
		return false
	}
	wh.Restarts++
	wh.State = WorkerAlive
	wh.LastHeartbeat = time.Now()
	ws.mu.Unlock()
	return true
}

func (ws *WorkerSupervisor) FailureSummary() map[string]int {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	result := map[string]int{
		"total_failures":   0,
		"total_restarts":   0,
		"degraded_workers": 0,
		"failed_workers":   0,
		"alive_workers":    0,
	}
	for _, wh := range ws.health {
		result["total_failures"] += wh.Failures
		result["total_restarts"] += wh.Restarts
		switch wh.State {
		case WorkerAlive:
			result["alive_workers"]++
		case WorkerDegraded:
			result["degraded_workers"]++
		case WorkerFailed:
			result["failed_workers"]++
		}
	}
	return result
}
