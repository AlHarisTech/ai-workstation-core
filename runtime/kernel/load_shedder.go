package kernel

import (
	"sync"
	"sync/atomic"
	"time"
)

type LoadState int32

const (
	LoadNormal       LoadState = 0
	LoadDegraded     LoadState = 1
	LoadProtected    LoadState = 2
	LoadEmergencyShed LoadState = 3
)

func (s LoadState) String() string {
	switch s {
	case LoadNormal:
		return "NORMAL"
	case LoadDegraded:
		return "DEGRADED"
	case LoadProtected:
		return "PROTECTED"
	case LoadEmergencyShed:
		return "EMERGENCY_SHED"
	default:
		return "UNKNOWN"
	}
}

type LoadShedder struct {
	state            atomic.Int32
	slaViolations    atomic.Int64
	slaViolationThreshold int64
	cooldownUntil    time.Time
	mu               sync.Mutex
}

func NewLoadShedder() *LoadShedder {
	ls := &LoadShedder{slaViolationThreshold: 5}
	ls.state.Store(int32(LoadNormal))
	return ls
}

func (ls *LoadShedder) State() LoadState {
	return LoadState(ls.state.Load())
}

func (ls *LoadShedder) RecordSLAViolation() {
	ls.slaViolations.Add(1)
	ls.mu.Lock()
	defer ls.mu.Unlock()

	v := ls.slaViolations.Load()
	switch {
	case v >= 20:
		ls.state.Store(int32(LoadEmergencyShed))
	case v >= 10:
		ls.state.Store(int32(LoadProtected))
	case v >= ls.slaViolationThreshold:
		ls.state.Store(int32(LoadDegraded))
	}
}

func (ls *LoadShedder) ShouldAccept(isCritical bool) bool {
	state := ls.State()
	switch state {
	case LoadNormal:
		return true
	case LoadDegraded:
		return true
	case LoadProtected:
		return isCritical
	case LoadEmergencyShed:
		return isCritical
	}
	return true
}

func (ls *LoadShedder) Saturation(used, max int) float64 {
	if max == 0 {
		return 0
	}
	return float64(used) / float64(max)
}

func (ls *LoadShedder) Evaluate(queueSize, queueMax int, workerUtil float64) LoadState {
	saturation := ls.Saturation(queueSize, queueMax)

	switch {
	case saturation > 0.9 || workerUtil > 0.95:
		ls.state.Store(int32(LoadEmergencyShed))
	case saturation > 0.7 || workerUtil > 0.85:
		ls.state.Store(int32(LoadProtected))
	case saturation > 0.5 || workerUtil > 0.7:
		ls.state.Store(int32(LoadDegraded))
	default:
		if ls.state.Load() >= int32(LoadDegraded) {
			ls.state.Store(int32(LoadNormal))
		}
	}

	return ls.State()
}

func (ls *LoadShedder) SLAViolations() int64 { return ls.slaViolations.Load() }
