package worker

import (
	"context"
	"fmt"
	"sync"

	"github.com/AlHarisTech/ai-workstation-core/runtime/executor"
	"github.com/AlHarisTech/ai-workstation-core/runtime/policy"
	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type WorkerPool struct {
	workers []*Worker
	wg      sync.WaitGroup
}

func NewWorkerPool(
	count int,
	queue <-chan *types.RequestContext,
	results chan<- *types.RequestContext,
	errors chan<- types.KernelEvent,
	policyEngine *policy.PolicyEngine,
	exec *executor.TimeoutWrapper,
	logger func(types.LogEntry),
	registry ToolRegistry,
) *WorkerPool {
	pool := &WorkerPool{
		workers: make([]*Worker, count),
	}

	for i := 0; i < count; i++ {
		pool.workers[i] = &Worker{
			ID:       fmt.Sprintf("wrk_%03d", i),
			queue:    queue,
			results:  results,
			errors:   errors,
			policy:   policyEngine,
			executor: exec,
			logger:   logger,
			pipeline: NewPipeline(registry),
		}
	}

	return pool
}

func (wp *WorkerPool) Start(ctx context.Context) {
	for _, w := range wp.workers {
		wp.wg.Add(1)
		go func(worker *Worker) {
			defer wp.wg.Done()
			worker.Run(ctx)
		}(w)
	}
}

func (wp *WorkerPool) Wait() {
	wp.wg.Wait()
}

func (wp *WorkerPool) WorkerCount() int {
	return len(wp.workers)
}
