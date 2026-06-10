package queue

import (
	"sync"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type FairnessViolation struct {
	Type        string   `json:"type"`
	SessionID   string   `json:"session_id"`
	WaitTimeMs  float64  `json:"wait_time_ms"`
	RiskSessions []string `json:"risk_sessions,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

type FairQueue struct {
	sessions       map[string]*sessionEntry
	order          []string
	pos            int
	mu             sync.Mutex
	maxSize        int
	total          int
	maxStarvation  time.Duration
	violationCb    func(FairnessViolation)
	lastCheck      time.Time
}

type sessionEntry struct {
	requests   []*types.RequestContext
	firstEnqueue time.Time
	totalServed  int64
	maxWait      time.Duration
}

func NewFairQueue(maxSize int) *FairQueue {
	return &FairQueue{
		sessions:      make(map[string]*sessionEntry),
		order:         make([]string, 0, 16),
		maxSize:       maxSize,
		maxStarvation: 30 * time.Second,
		lastCheck:     time.Now(),
		violationCb:   func(fv FairnessViolation) {},
	}
}

func (fq *FairQueue) SetViolationCallback(cb func(FairnessViolation)) {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	fq.violationCb = cb
}

func (fq *FairQueue) Enqueue(req *types.RequestContext) error {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	if fq.total >= fq.maxSize {
		return ErrQueueFull
	}

	sid := req.SessionID
	if sid == "" {
		sid = "_global"
	}

	entry, exists := fq.sessions[sid]
	if !exists {
		fq.order = append(fq.order, sid)
		entry = &sessionEntry{firstEnqueue: time.Now()}
		fq.sessions[sid] = entry
	}
	entry.requests = append(entry.requests, req)
	fq.total++

	fq.checkFairness()
	return nil
}

func (fq *FairQueue) Dequeue() *types.RequestContext {
	fq.mu.Lock()
	defer fq.mu.Unlock()

	if fq.total == 0 {
		return nil
	}

	startPos := fq.pos
	for {
		if len(fq.order) == 0 {
			return nil
		}

		sid := fq.order[fq.pos]
		entry := fq.sessions[sid]
		if entry != nil && len(entry.requests) > 0 {
			req := entry.requests[0]
			entry.requests = entry.requests[1:]
			waitTime := time.Since(req.TimestampStart)
			if waitTime > entry.maxWait {
				entry.maxWait = waitTime
			}
			entry.totalServed++
			fq.total--

			if len(entry.requests) == 0 {
				delete(fq.sessions, sid)
				fq.order = append(fq.order[:fq.pos], fq.order[fq.pos+1:]...)
				if fq.pos >= len(fq.order) {
					fq.pos = 0
				}
			} else {
				fq.pos = (fq.pos + 1) % len(fq.order)
			}
			return req
		}

		fq.pos = (fq.pos + 1) % len(fq.order)
		if fq.pos == startPos {
			break
		}
	}

	return nil
}

func (fq *FairQueue) checkFairness() {
	now := time.Now()
	if now.Sub(fq.lastCheck) < time.Second {
		return
	}
	fq.lastCheck = now

	for sid, entry := range fq.sessions {
		waitTime := now.Sub(entry.firstEnqueue)
		if waitTime > fq.maxStarvation/2 && waitTime <= fq.maxStarvation {
			fq.violationCb(FairnessViolation{
				Type:       "FAIRNESS_WARNING",
				SessionID:  sid,
				WaitTimeMs: float64(waitTime.Microseconds()) / 1000.0,
				Timestamp:  now,
			})
		}
		if waitTime > fq.maxStarvation {
			risk := fq.starvationRiskLocked()
			fq.violationCb(FairnessViolation{
				Type:        "FAIRNESS_VIOLATION",
				SessionID:    sid,
				WaitTimeMs:   float64(waitTime.Microseconds()) / 1000.0,
				RiskSessions: risk,
				Timestamp:    now,
			})
		}
	}
}

func (fq *FairQueue) starvationRiskLocked() []string {
	var risk []string
	maxPending := 0
	for _, entry := range fq.sessions {
		if len(entry.requests) > maxPending {
			maxPending = len(entry.requests)
		}
	}
	for sid, entry := range fq.sessions {
		if len(entry.requests) > 0 && maxPending > 3 && len(entry.requests) >= maxPending/2 {
			risk = append(risk, sid)
		}
	}
	return risk
}

func (fq *FairQueue) StarvationRisk() []string {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return fq.starvationRiskLocked()
}

func (fq *FairQueue) MaxStarvationMs() float64 {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	var maxMs float64
	for _, entry := range fq.sessions {
		ms := float64(entry.maxWait.Microseconds()) / 1000.0
		if ms > maxMs {
			maxMs = ms
		}
	}
	return maxMs
}

func (fq *FairQueue) Size() int {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return fq.total
}

func (fq *FairQueue) SessionCount() int {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	return len(fq.sessions)
}

func (fq *FairQueue) SessionSizes() map[string]int {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	result := make(map[string]int, len(fq.sessions))
	for sid, entry := range fq.sessions {
		result[sid] = len(entry.requests)
	}
	return result
}
