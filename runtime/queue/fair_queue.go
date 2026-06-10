package queue

import (
	"sync"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type FairQueue struct {
	sessions map[string][]*types.RequestContext
	order    []string
	pos      int
	mu       sync.Mutex
	maxSize  int
	total    int
}

func NewFairQueue(maxSize int) *FairQueue {
	return &FairQueue{
		sessions: make(map[string][]*types.RequestContext),
		order:    make([]string, 0, 16),
		maxSize:  maxSize,
	}
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

	if _, exists := fq.sessions[sid]; !exists {
		fq.order = append(fq.order, sid)
	}
	fq.sessions[sid] = append(fq.sessions[sid], req)
	fq.total++
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
		if len(fq.sessions[sid]) > 0 {
			req := fq.sessions[sid][0]
			fq.sessions[sid] = fq.sessions[sid][1:]
			fq.total--

			if len(fq.sessions[sid]) == 0 {
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
	for sid, reqs := range fq.sessions {
		result[sid] = len(reqs)
	}
	return result
}

func (fq *FairQueue) StarvationRisk() []string {
	fq.mu.Lock()
	defer fq.mu.Unlock()
	var risk []string
	maxPending := 0
	for _, reqs := range fq.sessions {
		if len(reqs) > maxPending {
			maxPending = len(reqs)
		}
	}
	for sid, reqs := range fq.sessions {
		if len(reqs) > 0 && maxPending > 3 && len(reqs) >= maxPending/2 {
			risk = append(risk, sid)
		}
	}
	return risk
}
