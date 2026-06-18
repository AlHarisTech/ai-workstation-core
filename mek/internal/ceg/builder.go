package ceg

import (
	"fmt"
	"sort"

	"github.com/anomalyco/mek/pkg/types"
)

type BuildError struct {
	Code    string
	Message string
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("CEG build failed: [%s] %s", e.Code, e.Message)
}

func Build(rir *types.RIR) (*types.CEG, error) {
	ceg := &types.CEG{
		NodeMap:  make(map[string]*types.CEGNode),
		InDegree: make(map[string]int),
	}

	// Build nodes
	for _, u := range rir.Units {
		activation := u.Activation
		if activation.Requires == nil {
			activation.Requires = []string{}
		}
		if activation.Optional == nil {
			activation.Optional = []string{}
		}

		node := &types.CEGNode{
			ID:         u.ID,
			Type:       u.Type,
			Activation: activation,
			Gate:       u.Gate,
			Binding:    u.Binding,
			Scheduling: u.Scheduling,
		}
		ceg.Nodes = append(ceg.Nodes, node)
		ceg.NodeMap[u.ID] = node
		ceg.InDegree[u.ID] = 0
	}

	// Build edges
	for _, edge := range rir.Graph.DAG.Edges {
		cegEdge := &types.CEGEdge{
			From: edge.From,
			To:   edge.To,
			Type: edge.Type,
		}
		ceg.Edges = append(ceg.Edges, cegEdge)

		if from, ok := ceg.NodeMap[edge.From]; ok {
			from.Successors = append(from.Successors, edge.To)
		}
		if to, ok := ceg.NodeMap[edge.To]; ok {
			to.Predecessors = append(to.Predecessors, edge.From)
			ceg.InDegree[edge.To]++
		}
	}

	// Compute topological layers (must happen before depth validation)
	topoSort(ceg, rir)

	// Validate (includes V-004: max_depth ≤ 128, which depends on topoSort)
	if err := validate(ceg); err != nil {
		return nil, err
	}

	return ceg, nil
}

func validate(ceg *types.CEG) error {
	// V-001: No cycles
	if hasCycle(ceg) {
		return &BuildError{Code: "CYCLES_DETECTED", Message: "CEG contains cycles"}
	}

	// V-003: indegree ≤ 1024
	for _, node := range ceg.Nodes {
		if len(node.Predecessors) > 1024 {
			return &BuildError{
				Code:    "INDEGREE_EXCEEDED",
				Message: fmt.Sprintf("node %s has indegree %d (max 1024)", node.ID, len(node.Predecessors)),
			}
		}
	}

	// V-004: depth ≤ 128
	if ceg.MaxDepth > 128 {
		return &BuildError{
			Code:    "MAX_DEPTH_EXCEEDED",
			Message: fmt.Sprintf("CEG depth is %d (max 128)", ceg.MaxDepth),
		}
	}

	// V-002: All nodes reachable (no orphans disconnected from root set)
	reachable := reachableSet(ceg)
	if len(reachable) != len(ceg.Nodes) {
		return &BuildError{
			Code:    "UNREACHABLE_NODES",
			Message: fmt.Sprintf("CEG has %d unreachable nodes", len(ceg.Nodes)-len(reachable)),
		}
	}

	return nil
}

func hasCycle(ceg *types.CEG) bool {
	visited := make(map[string]int) // 0=unvisited, 1=visiting, 2=done
	var dfs func(id string) bool
	dfs = func(id string) bool {
		state := visited[id]
		if state == 1 {
			return true // back edge = cycle
		}
		if state == 2 {
			return false
		}
		visited[id] = 1
		node := ceg.NodeMap[id]
		if node != nil {
			for _, succ := range node.Successors {
				if dfs(succ) {
					return true
				}
			}
		}
		visited[id] = 2
		return false
	}
	for _, node := range ceg.Nodes {
		if visited[node.ID] == 0 {
			if dfs(node.ID) {
				return true
			}
		}
	}
	return false
}

func reachableSet(ceg *types.CEG) map[string]bool {
	reachable := make(map[string]bool)
	var dfs func(id string)
	dfs = func(id string) {
		if reachable[id] {
			return
		}
		reachable[id] = true
		node := ceg.NodeMap[id]
		if node != nil {
			for _, succ := range node.Successors {
				dfs(succ)
			}
		}
	}
	// Start from roots (indegree = 0)
	for _, node := range ceg.Nodes {
		if ceg.InDegree[node.ID] == 0 {
			dfs(node.ID)
		}
	}
	return reachable
}

func topoSort(ceg *types.CEG, rir *types.RIR) {
	// Kahn's algorithm
	inDeg := make(map[string]int)
	for k, v := range ceg.InDegree {
		inDeg[k] = v
	}

	queue := make([]string, 0)
	for _, node := range ceg.Nodes {
		if inDeg[node.ID] == 0 {
			queue = append(queue, node.ID)
		}
	}
	sort.Strings(queue) // deterministic

	layer := 0
	nodeLayer := make(map[string]int)

	for len(queue) > 0 {
		layerSize := len(queue)
		currentLayer := make([]string, layerSize)
		copy(currentLayer, queue)

		ceg.Layers = append(ceg.Layers, currentLayer)
		ceg.MaxDepth = layer + 1

		for i := 0; i < layerSize; i++ {
			id := queue[0]
			queue = queue[1:]
			nodeLayer[id] = layer

			node := ceg.NodeMap[id]
			if node != nil {
				sort.Strings(node.Successors) // deterministic
				for _, succ := range node.Successors {
					inDeg[succ]--
					if inDeg[succ] == 0 {
						queue = append(queue, succ)
					}
				}
			}
		}
		sort.Strings(queue) // deterministic
		layer++
	}

	// Assign layers to nodes
	for _, node := range ceg.Nodes {
		node.Layer = nodeLayer[node.ID]
	}
}

// EvaluateActivation evaluates whether a blocked node can become READY.
func EvaluateActivation(node *types.CEGNode, sm *types.StatusMap) bool {
	switch node.Activation.Condition {
	case "all_success":
		for _, req := range node.Activation.Requires {
			status, ok := sm.Get(req)
			if !ok || status != types.StatusSuccess {
				return false
			}
		}
		return true

	case "any_success":
		candidates := append([]string{}, node.Activation.Requires...)
		candidates = append(candidates, node.Activation.Optional...)
		for _, c := range candidates {
			status, ok := sm.Get(c)
			if ok && status == types.StatusSuccess {
				return true
			}
		}
		return false

	default:
		// conditional — delegated to predicate evaluation
		if len(node.Activation.Condition) > len("conditional:") &&
			node.Activation.Condition[:12] == "conditional:" {
			// In MEK v1, arbitrary predicates are not evaluated.
			// The node stays BLOCKED until deadline triggers TERMINATE.
			return false
		}
		return false
	}
}

// ShouldSkip determines if a blocked node should be SKIPPED.
func ShouldSkip(node *types.CEGNode, sm *types.StatusMap) bool {
	switch node.Activation.Condition {
	case "all_success":
		// A single failed/skipped/terminated required predecessor → SKIP
		for _, req := range node.Activation.Requires {
			status, ok := sm.Get(req)
			if ok && status.IsTerminal() && status != types.StatusSuccess {
				return true
			}
		}
		return false

	case "any_success":
		// ALL candidates terminal AND NONE SUCCESS → SKIP
		candidates := append([]string{}, node.Activation.Requires...)
		candidates = append(candidates, node.Activation.Optional...)
		if len(candidates) == 0 {
			return false // no candidates, never skip
		}
		for _, c := range candidates {
			status, ok := sm.Get(c)
			if !ok || !status.IsTerminal() {
				return false // at least one not terminal yet
			}
			if status == types.StatusSuccess {
				return false // at least one succeeded
			}
		}
		return true // all terminal, none success

	default:
		// conditional — never auto-skip
		return false
	}
}
