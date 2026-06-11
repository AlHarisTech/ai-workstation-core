package orchestratorv1

type Decider interface {
	Decide(event *OrchestratorEvent) RoutingDecision
}

type DefaultDecider struct{}

func NewDefaultDecider() *DefaultDecider {
	return &DefaultDecider{}
}

func (d *DefaultDecider) Decide(event *OrchestratorEvent) RoutingDecision {
	decision := RoutingDecision{
		Postgres: true,
		ChromaDB: true,
	}

	op := event.Execution.Operation
	server := event.Execution.Server

	multiStepOps := map[string]bool{
		"commit": true, "push": true, "create_release": true,
		"create_pr": true, "write": true,
	}
	agentOps := map[string]bool{
		"resolve": true, "query": true, "create_issue": true,
	}

	if multiStepOps[op] || server == "langgraph" {
		decision.LangGraph = true
	}
	if agentOps[op] || server == "crewai" {
		decision.CrewAI = true
	}

	return decision
}
