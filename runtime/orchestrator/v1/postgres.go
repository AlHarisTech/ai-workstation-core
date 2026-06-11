package orchestratorv1

import (
	"encoding/json"
	"fmt"
	"log"
)

type PostgresStore interface {
	StoreExecution(event *OrchestratorEvent) error
}

type PostgresConfig struct {
	ConnString string
	TableName  string
}

type PostgresAdapter struct {
	config PostgresConfig
}

func NewPostgresAdapter(config PostgresConfig) *PostgresAdapter {
	return &PostgresAdapter{config: config}
}

func (p *PostgresAdapter) StoreExecution(event *OrchestratorEvent) error {
	if p.config.ConnString == "" {
		log.Printf("[orchestrator:postgres] no connection string configured, logging execution %s", event.TraceID)
		p.logEvent(event)
		return nil
	}
	return p.store(event)
}

func (p *PostgresAdapter) store(event *OrchestratorEvent) error {
	resultJSON, err := json.Marshal(event.Execution.Result)
	if err != nil {
		return fmt.Errorf("marshal result: %w", err)
	}
	table := p.config.TableName
	if table == "" {
		table = "executions"
	}
	sql := fmt.Sprintf(
		`INSERT INTO %s (trace_id, session_id, server, operation, result, created_at) VALUES ($1, $2, $3, $4, $5, NOW())`,
		table,
	)
	log.Printf("[orchestrator:postgres] would execute: %s", sql)
	log.Printf("  trace_id=%s session_id=%s server=%s operation=%s result=%s",
		event.TraceID, event.Context.SessionID, event.Execution.Server,
		event.Execution.Operation, string(resultJSON))
	return nil
}

func (p *PostgresAdapter) logEvent(event *OrchestratorEvent) {
	resultJSON, _ := json.Marshal(event.Execution.Result)
	log.Printf("[orchestrator:postgres] event trace_id=%s server=%s op=%s result=%s",
		event.TraceID, event.Execution.Server, event.Execution.Operation, string(resultJSON))
}
