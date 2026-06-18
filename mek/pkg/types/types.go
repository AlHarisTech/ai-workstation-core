package types

import (
	"sync"
	"time"
)

// ─── RIR (Runtime Intermediate Representation) ───

type RIR struct {
	Meta          RIRMeta           `json:"meta" yaml:"meta"`
	ExecutionPlan ExecutionPlan     `json:"execution_plan" yaml:"execution_plan"`
	Units         []Unit            `json:"units" yaml:"units"`
	Graph         Graph             `json:"graph" yaml:"graph"`
	Assertions    []Assertion       `json:"assertions" yaml:"assertions"`
	FailureModes  []FailureMode     `json:"failure_modes" yaml:"failure_modes"`
	IsolationRegions []IsolationBoundary `json:"isolated_regions,omitempty" yaml:"isolated_regions"`
	Checkpoints   []Checkpoint      `json:"checkpoints,omitempty" yaml:"checkpoints"`
	Handoff       Handoff           `json:"handoff,omitempty" yaml:"handoff"`
}

type RIRMeta struct {
	SchemaVersion  string `json:"schema_version" yaml:"schema_version"`
	SpecHash       string `json:"spec_hash" yaml:"spec_hash"`
	CompilationID  string `json:"compilation_id" yaml:"compilation_id"`
	CompiledAt     string `json:"compiled_at" yaml:"compiled_at"`
	SourceSpec     string `json:"source_spec" yaml:"source_spec"`
	CompilerVersion string `json:"compiler_version" yaml:"compiler_version"`
}

type ExecutionPlan struct {
	SchedulingModel    string `json:"scheduling_model" yaml:"scheduling_model"`
	ExecutionStrategy  string `json:"execution_strategy" yaml:"execution_strategy"`
	MaxParallelism     int    `json:"max_parallelism" yaml:"max_parallelism"`
	FailStrategy       string `json:"fail_strategy" yaml:"fail_strategy"`
	ExecutionMode      string `json:"execution_mode" yaml:"execution_mode"`
}

// ─── Units ───

type Unit struct {
	ID           string         `json:"id" yaml:"id"`
	Type         UnitType       `json:"type" yaml:"type"`
	AgentRef     string         `json:"agent_ref,omitempty" yaml:"agent_ref,omitempty"`
	Binding      Binding        `json:"binding" yaml:"binding"`
	Dependencies []string       `json:"dependencies" yaml:"dependencies"`
	DataFlow     DataFlow       `json:"data_flow" yaml:"data_flow"`
	Validation   Validation     `json:"validation" yaml:"validation"`
	Scheduling   Scheduling     `json:"scheduling" yaml:"scheduling"`
	Context      UnitContext    `json:"context" yaml:"context"`
	Governance   Governance     `json:"governance" yaml:"governance"`
	Activation   Activation     `json:"activation" yaml:"activation"`
	Gate         *GateConfig    `json:"gate,omitempty" yaml:"gate,omitempty"`
}

type UnitType string

const (
	UnitAgent      UnitType = "agent"
	UnitCapability UnitType = "capability"
	UnitTask       UnitType = "task"
	UnitGate       UnitType = "gate"
	UnitCheckpoint UnitType = "checkpoint"
)

type Activation struct {
	Condition string   `json:"condition" yaml:"condition"`
	Requires  []string `json:"requires" yaml:"requires"`
	Optional  []string `json:"optional,omitempty" yaml:"optional,omitempty"`
}

type GateConfig struct {
	Branches []GateBranch `json:"branches" yaml:"branches"`
	Default  string       `json:"default" yaml:"default"`
}

type GateBranch struct {
	Condition string `json:"condition" yaml:"condition"`
	Target    string `json:"target" yaml:"target"`
}

type Binding struct {
	Contract  string `json:"contract" yaml:"contract"`
	Isolation string `json:"isolation" yaml:"isolation"`
}

type DataFlow struct {
	Inputs  []DataFlowInput `json:"inputs,omitempty" yaml:"inputs,omitempty"`
	Outputs []string        `json:"outputs" yaml:"outputs"`
}

type DataFlowInput struct {
	Key      string         `json:"key" yaml:"key"`
	From     DataFlowSource `json:"from" yaml:"from"`
	Contract string         `json:"contract" yaml:"contract"`
}

type DataFlowSource struct {
	Unit   string `json:"unit" yaml:"unit"`
	Output string `json:"output" yaml:"output"`
}

type Scheduling struct {
	Priority int          `json:"priority" yaml:"priority"`
	Deadline string       `json:"deadline,omitempty" yaml:"deadline,omitempty"`
	Retry    *RetryConfig `json:"retry,omitempty" yaml:"retry,omitempty"`
}

type RetryConfig struct {
	MaxAttempts int    `json:"max_attempts" yaml:"max_attempts"`
	Backoff     string `json:"backoff" yaml:"backoff"`
}

type Validation struct {
	Preconditions  []string `json:"preconditions" yaml:"preconditions"`
	Postconditions []string `json:"postconditions" yaml:"postconditions"`
	Invariants     []string `json:"invariants" yaml:"invariants"`
	FailureModes   []string `json:"failure_modes" yaml:"failure_modes"`
}

type UnitContext struct {
	Mode      string   `json:"mode" yaml:"mode"`
	MaxTokens int      `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	Tools     []string `json:"tools" yaml:"tools"`
}

type Governance struct {
	RequiredApprovals []string `json:"required_approvals" yaml:"required_approvals"`
	ChangeScope       string   `json:"change_scope" yaml:"change_scope"`
}

// ─── Graph ───

type Graph struct {
	DAG              DAG                `json:"dag" yaml:"dag"`
	Cycles           [][]string         `json:"cycles" yaml:"cycles"`
	IsolatedRegions  []IsolatedRegion  `json:"isolated_regions,omitempty" yaml:"isolated_regions"`
}

type DAG struct {
	Nodes []string  `json:"nodes" yaml:"nodes"`
	Edges []Edge    `json:"edges" yaml:"edges"`
}

type Edge struct {
	From string `json:"from" yaml:"from"`
	To   string `json:"to" yaml:"to"`
	Type string `json:"type" yaml:"type"`
}

type IsolatedRegion struct {
	Units  []string `json:"units" yaml:"units"`
	Reason string   `json:"reason" yaml:"reason"`
}

// ─── Assertions & Failure Modes ───

type Assertion struct {
	ID         string `json:"id" yaml:"id"`
	Type       string `json:"type" yaml:"type"`
	Predicate  string `json:"predicate" yaml:"predicate"`
	OnFailure  string `json:"on_failure" yaml:"on_failure"`
}

type FailureMode struct {
	ID         string `json:"id" yaml:"id"`
	Condition  string `json:"condition" yaml:"condition"`
	Action     string `json:"action" yaml:"action"`
	MaxRetries int    `json:"max_retries" yaml:"max_retries"`
}

// ─── Isolation ───

type IsolationBoundary struct {
	ID          string   `json:"id" yaml:"id"`
	Units       []string `json:"units" yaml:"units"`
	Constraints []string `json:"constraints,omitempty" yaml:"constraints,omitempty"`
	Reason      string   `json:"reason" yaml:"reason"`
}

// ─── Checkpoints & Handoff ───

type Checkpoint struct {
	AfterUnit string   `json:"after_unit" yaml:"after_unit"`
	Save      []string `json:"save" yaml:"save"`
	Label     string   `json:"label" yaml:"label"`
}

type Handoff struct {
	OutputArtifacts []string `json:"output_artifacts" yaml:"output_artifacts"`
	Evidence        []string `json:"evidence" yaml:"evidence"`
	NextSpec        string   `json:"next_spec,omitempty" yaml:"next_spec,omitempty"`
}

// ─── CEG (Canonical Execution Graph) ───

type CEG struct {
	Nodes    []*CEGNode
	Edges    []*CEGEdge
	NodeMap  map[string]*CEGNode
	InDegree map[string]int
	Layers   [][]string
	MaxDepth int
}

type CEGNode struct {
	ID          string
	Type        UnitType
	Activation  Activation
	Gate        *GateConfig
	Binding     Binding
	Scheduling  Scheduling
	RegionID    string
	Predecessors []string
	Successors   []string
	Layer        int
}

type CEGEdge struct {
	From string
	To   string
	Type string
}

// ─── Node Status ───

type NodeStatus string

const (
	StatusBlocked    NodeStatus = "BLOCKED"
	StatusReady      NodeStatus = "READY"
	StatusRunning    NodeStatus = "RUNNING"
	StatusSuccess    NodeStatus = "SUCCESS"
	StatusFailure    NodeStatus = "FAILURE"
	StatusSkipped    NodeStatus = "SKIPPED"
	StatusTerminated NodeStatus = "TERMINATED"
)

func (s NodeStatus) IsTerminal() bool {
	return s == StatusSuccess || s == StatusFailure || s == StatusSkipped || s == StatusTerminated
}

// ─── Status Map ───

type NodeState struct {
	Status    NodeStatus
	Outputs   map[string]interface{}
	Artifacts []string
}

type StatusMap struct {
	mu     sync.RWMutex
	states map[string]*NodeState
}

func NewStatusMap() *StatusMap {
	return &StatusMap{states: make(map[string]*NodeState)}
}

func (sm *StatusMap) Get(nodeID string) (NodeStatus, bool) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	s, ok := sm.states[nodeID]
	if !ok {
		return "", false
	}
	return s.Status, true
}

func (sm *StatusMap) GetState(nodeID string) *NodeState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.states[nodeID]
}

func (sm *StatusMap) Set(nodeID string, status NodeStatus) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	if s, ok := sm.states[nodeID]; ok {
		s.Status = status
	}
}

func (sm *StatusMap) SetState(nodeID string, state *NodeState) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.states[nodeID] = state
}

func (sm *StatusMap) Init(nodeID string, status NodeStatus) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.states[nodeID] = &NodeState{Status: status, Outputs: make(map[string]interface{})}
}

func (sm *StatusMap) All() map[string]*NodeState {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	out := make(map[string]*NodeState, len(sm.states))
	for k, v := range sm.states {
		out[k] = v
	}
	return out
}

func (sm *StatusMap) AllTerminal(nodeIDs []string) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	for _, id := range nodeIDs {
		s, ok := sm.states[id]
		if !ok || !s.Status.IsTerminal() {
			return false
		}
	}
	return true
}

// ─── Execution Result ───

type ExecutionResult struct {
	Status     NodeStatus              `json:"status"`
	Outputs    map[string]interface{}  `json:"outputs,omitempty"`
	Artifacts  []string                `json:"artifacts,omitempty"`
	Escalation bool                    `json:"escalation,omitempty"`
	SelectedBranch string              `json:"selected_branch,omitempty"`
	Metrics    ExecutionMetrics        `json:"metrics"`
}

type ExecutionMetrics struct {
	DurationMs   int  `json:"duration_ms"`
	TokensUsed   *int `json:"tokens_used,omitempty"`
}

// ─── Execution Context ───

type ExecutionContext struct {
	ExecutionID    string
	NodeID         string
	RegionID       string
	TimeoutMs      int
	FilesystemScope string
	NetworkScope   string
}

// ─── MEK Output ───

type MEKOutput struct {
	StatusMap map[string]*NodeState `json:"status_map"`
	Metrics   MEKMetrics            `json:"metrics"`
}

type MEKMetrics struct {
	TotalDurationMs    int  `json:"total_duration_ms"`
	NodesExecuted      int  `json:"nodes_executed"`
	NodesFailed        int  `json:"nodes_failed"`
	WavesCompleted     int  `json:"waves_completed"`
	EscalationRequested bool `json:"escalation_requested"`
}

// ─── Isolation Ordering ───

var IsolationOrder = map[string]int{
	"inline":    0,
	"process":   1,
	"container": 2,
}

// RegionToAdapterIsolation maps region isolation requirements to minimum adapter isolation.
var RegionToAdapterIsolation = map[string]string{
	"no_shared_state": "process",
	"no_network":      "process",
	"no_filesystem":   "process",
	"full_sandbox":    "container",
}

// ─── Helpers ───

func Contains[E comparable](s []E, v E) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

func MapKeys[K comparable, V any](m map[K]V) []K {
	keys := make([]K, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func DeadlineDuration(deadline string) (time.Duration, error) {
	if deadline == "" {
		return 0, nil
	}
	t, err := time.Parse(time.RFC3339, deadline)
	if err != nil {
		return 0, err
	}
	d := time.Until(t)
	if d < 0 {
		return 0, nil
	}
	return d, nil
}
