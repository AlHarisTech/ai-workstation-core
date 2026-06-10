package worker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/executor"
	"github.com/AlHarisTech/ai-workstation-core/runtime/policy"
	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

type WorkerPool struct {
	workers    []*Worker
	wg         sync.WaitGroup
	supervisor *WorkerSupervisor
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
	supervisor := NewWorkerSupervisor()
	pool := &WorkerPool{
		workers:    make([]*Worker, count),
		supervisor: supervisor,
	}

	for i := 0; i < count; i++ {
		id := fmt.Sprintf("wrk_%03d", i)
		health := supervisor.Register(id)
		pool.workers[i] = &Worker{
			ID:         id,
			queue:      queue,
			results:    results,
			errors:     errors,
			policy:     policyEngine,
			executor:   exec,
			logger:     logger,
			pipeline:   NewPipeline(registry),
			supervisor: supervisor,
			health:     health,
			heartbeat:  5 * time.Second,
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

func (wp *WorkerPool) Supervisor() *WorkerSupervisor {
	return wp.supervisor
}
