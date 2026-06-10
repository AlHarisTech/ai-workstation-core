package queue

import (
	"errors"
	"sync/atomic"

	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

var ErrQueueFull = errors.New("QUEUE_FULL: backpressure limit reached")

type QueueMetrics struct {
	Enqueued  atomic.Int64
	Dequeued  atomic.Int64
	Rejected  atomic.Int64
	TimedOut  atomic.Int64
}

type RequestQueue struct {
	ch      chan *types.RequestContext
	maxSize int
	Metrics *QueueMetrics
}

func NewRequestQueue(maxSize int) *RequestQueue {
	return &RequestQueue{
		ch:      make(chan *types.RequestContext, maxSize),
		maxSize: maxSize,
		Metrics: &QueueMetrics{},
	}
}

func (q *RequestQueue) Enqueue(req *types.RequestContext) error {
	select {
	case q.ch <- req:
		q.Metrics.Enqueued.Add(1)
		return nil
	default:
		q.Metrics.Rejected.Add(1)
		return ErrQueueFull
	}
}

func (q *RequestQueue) Chan() <-chan *types.RequestContext {
	return q.ch
}

func (q *RequestQueue) Size() int {
	return len(q.ch)
}

func (q *RequestQueue) MaxSize() int {
	return q.maxSize
}

func (q *RequestQueue) IsFull() bool {
	return len(q.ch) >= q.maxSize
}

func (q *RequestQueue) Close() {
	close(q.ch)
}
