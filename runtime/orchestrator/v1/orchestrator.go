package orchestratorv1

import (
	"log"
	"sync"
)

type Orchestrator struct {
	decider   Decider
	postgres  PostgresStore
	chroma    ChromaStore
	langGraph LangGraphRunner
	crewAI    CrewAIRunner
}

type OrchestratorOption func(*Orchestrator)

func WithDecider(d Decider) OrchestratorOption {
	return func(o *Orchestrator) { o.decider = d }
}

func WithPostgres(p PostgresStore) OrchestratorOption {
	return func(o *Orchestrator) { o.postgres = p }
}

func WithChroma(c ChromaStore) OrchestratorOption {
	return func(o *Orchestrator) { o.chroma = c }
}

func WithLangGraph(l LangGraphRunner) OrchestratorOption {
	return func(o *Orchestrator) { o.langGraph = l }
}

func WithCrewAI(c CrewAIRunner) OrchestratorOption {
	return func(o *Orchestrator) { o.crewAI = c }
}

func NewOrchestrator(opts ...OrchestratorOption) *Orchestrator {
	o := &Orchestrator{
		decider:   NewDefaultDecider(),
		postgres:  NewPostgresAdapter(PostgresConfig{}),
		chroma:    NewChromaAdapter(ChromaConfig{}),
		langGraph: NewLangGraphAdapter(LangGraphConfig{}),
		crewAI:    NewCrewAIAdapter(CrewAIConfig{}),
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

func (o *Orchestrator) Process(event *OrchestratorEvent) *OrchestratorResponse {
	resp := &OrchestratorResponse{
		TraceID: event.TraceID,
		Status:  "completed",
		Result:  event.Execution.Result,
	}

	decision := o.decider.Decide(event)

	var mu sync.Mutex
	var wg sync.WaitGroup
	triggered := make(map[string]bool)

	safeRun := func(name string, fn func() error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[orchestrator] panic in %s: %v", name, r)
			}
		}()
		if err := fn(); err != nil {
			log.Printf("[orchestrator] %s failed (non-blocking): %v", name, err)
			return
		}
		mu.Lock()
		triggered[name] = true
		mu.Unlock()
	}

	if decision.Postgres {
		wg.Add(1)
		go func() {
			defer wg.Done()
			safeRun("postgres", func() error {
				return o.postgres.StoreExecution(event)
			})
		}()
	}

	if decision.ChromaDB {
		wg.Add(1)
		go func() {
			defer wg.Done()
			safeRun("chroma", func() error {
				return o.chroma.StoreMemory(event)
			})
		}()
	}

	if decision.LangGraph {
		wg.Add(1)
		go func() {
			defer wg.Done()
			safeRun("langgraph", func() error {
				_, err := o.langGraph.Execute(event)
				return err
			})
		}()
	}

	if decision.CrewAI {
		wg.Add(1)
		go func() {
			defer wg.Done()
			safeRun("crewai", func() error {
				_, err := o.crewAI.Execute(event)
				return err
			})
		}()
	}

	wg.Wait()

	for s := range triggered {
		resp.SystemsTriggered = append(resp.SystemsTriggered, s)
	}
	if resp.SystemsTriggered == nil {
		resp.SystemsTriggered = []string{}
	}

	return resp
}
