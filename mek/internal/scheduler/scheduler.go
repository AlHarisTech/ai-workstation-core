package scheduler

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/anomalyco/mek/internal/ceg"
	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/pkg/types"
)

type Scheduler struct {
	ceg   *types.CEG
	sm    *types.StatusMap
	comm  *commit.Engine
	Waves [][][]string // Waves[layer][waveIndex] = []nodeID
}

func New(c *types.CEG, sm *types.StatusMap, comm *commit.Engine) *Scheduler {
	return &Scheduler{
		ceg:  c,
		sm:   sm,
		comm: comm,
	}
}

// ComputeWaves partitions each topological layer into conflict-free waves.
// Deterministic: same CEG → same wave structure (M-006).
func (s *Scheduler) ComputeWaves(rir *types.RIR) error {
	s.Waves = make([][][]string, len(s.ceg.Layers))

	for layerIdx, layer := range s.ceg.Layers {
		if len(layer) == 0 {
			continue
		}

		// Build conflict graph for this layer
		conflicts := buildConflictGraph(layer, s.ceg, rir)

		// Color the conflict graph (deterministic, node-id-ordered)
		waves := colorGraph(layer, conflicts)

		s.Waves[layerIdx] = waves
	}
	return nil
}

func buildConflictGraph(layer []string, ceg *types.CEG, rir *types.RIR) map[string]map[string]bool {
	conflicts := make(map[string]map[string]bool)
	for _, id := range layer {
		conflicts[id] = make(map[string]bool)
	}

	// Get side_effect_surface for each node
	surfaces := make(map[string][]string)
	for _, u := range rir.Units {
		surfaces[u.ID] = collectSurfaces(u, rir)
	}

	for i := 0; i < len(layer); i++ {
		for j := i + 1; j < len(layer); j++ {
			a, b := layer[i], layer[j]
			if hasOverlap(surfaces[a], surfaces[b]) {
				conflicts[a][b] = true
				conflicts[b][a] = true
			}
			// DATA_FLOW conflict: shared data contract
			if hasDataFlowConflict(a, b, rir) {
				conflicts[a][b] = true
				conflicts[b][a] = true
			}
		}
	}
	return conflicts
}

func collectSurfaces(u types.Unit, rir *types.RIR) []string {
	var surfaces []string
	// Look up the unit's side_effect_surface from data_flow contracts
	for _, iso := range rir.IsolationRegions {
		if types.Contains(iso.Units, u.ID) {
			surfaces = append(surfaces, iso.Units...)
		}
	}
	return surfaces
}

func hasOverlap(a, b []string) bool {
	for _, sa := range a {
		for _, sb := range b {
			if sa == sb {
				return true
			}
		}
	}
	return false
}

func hasDataFlowConflict(a, b string, rir *types.RIR) bool {
	// Two nodes conflict if they share a data contract reference
	contractsA := make(map[string]bool)
	contractsB := make(map[string]bool)
	for _, u := range rir.Units {
		for _, dfi := range u.DataFlow.Inputs {
			if u.ID == a {
				contractsA[dfi.Contract] = true
			}
			if u.ID == b {
				contractsB[dfi.Contract] = true
			}
		}
	}
	for k := range contractsA {
		if contractsB[k] {
			return true
		}
	}
	return false
}

func colorGraph(nodes []string, conflicts map[string]map[string]bool) [][]string {
	sorted := make([]string, len(nodes))
	copy(sorted, nodes)
	sort.Strings(sorted) // deterministic (M-008)

	colors := make(map[string]int)
	for _, n := range sorted {
		used := make(map[int]bool)
		for neighbor := range conflicts[n] {
			if c, ok := colors[neighbor]; ok {
				used[c] = true
			}
		}
		c := 0
		for used[c] {
			c++
		}
		colors[n] = c
	}

	// Group by color
	maxColor := 0
	for _, c := range colors {
		if c > maxColor {
			maxColor = c
		}
	}

	waves := make([][]string, maxColor+1)
	for _, n := range sorted {
		waves[colors[n]] = append(waves[colors[n]], n)
	}

	// Sort each wave for determinism
	for i := range waves {
		sort.Strings(waves[i])
	}

	return waves
}

// InitStatusMap initializes node states. Root nodes (indegree=0) → READY.
// Root conditional nodes with unsatisfiable predicate → SKIPPED.
func (s *Scheduler) InitStatusMap() {
	for _, node := range s.ceg.Nodes {
		if s.ceg.InDegree[node.ID] == 0 {
			if !ceg.EvaluateActivation(node, s.sm) {
				s.comm.Commit(node.ID, types.StatusSkipped, nil)
			} else {
				s.comm.Commit(node.ID, types.StatusReady, nil)
			}
		} else {
			s.comm.Commit(node.ID, types.StatusBlocked, nil)
		}
	}
}

// Recompute evaluates all BLOCKED nodes and transitions them to READY or SKIPPED.
// Deterministic: node-id-ordered evaluation (M-008).
func (s *Scheduler) Recompute() {
	blocked := s.collectBlocked()
	sort.Strings(blocked) // deterministic

	for _, nodeID := range blocked {
		node := s.ceg.NodeMap[nodeID]
		if node == nil {
			continue
		}

		if ceg.EvaluateActivation(node, s.sm) {
			s.comm.Commit(nodeID, types.StatusReady, nil)
		} else if ceg.ShouldSkip(node, s.sm) {
			s.comm.Commit(nodeID, types.StatusSkipped, nil)
		}
		// conditional nodes stay BLOCKED — deadline handles termination
	}
}

func (s *Scheduler) collectBlocked() []string {
	var blocked []string
	all := s.sm.All()
	for id, state := range all {
		if state.Status == types.StatusBlocked {
			blocked = append(blocked, id)
		}
	}
	return blocked
}

// Claim returns all READY nodes in a wave and transitions them to RUNNING.
func (s *Scheduler) Claim(wave []string) ([]string, error) {
	var ready []string
	for _, nodeID := range wave {
		status, ok := s.sm.Get(nodeID)
		if ok && status == types.StatusReady {
			if err := s.comm.Commit(nodeID, types.StatusRunning, nil); err != nil {
				return nil, fmt.Errorf("claim %s: %w", nodeID, err)
			}
			ready = append(ready, nodeID)
		}
	}
	return ready, nil
}

// WaitWave blocks until all nodes in a wave are terminal, or deadline expires.
// Deadline is measured from wave start, covering BLOCKED conditional nodes.
func (s *Scheduler) WaitWave(ctx context.Context, wave []string, deadline time.Duration) error {
	if deadline <= 0 {
		// No deadline — wait indefinitely
		for !s.sm.AllTerminal(wave) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
		}
		return nil
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	for !s.sm.AllTerminal(wave) {
		select {
		case <-deadlineCtx.Done():
			// Deadline expired: terminate all non-terminal nodes in wave.
			// Wave deadlines are COLLECTIVE (MEK §3.3 WAIT_WAVE).
			for _, nodeID := range wave {
				status, ok := s.sm.Get(nodeID)
				if ok && !status.IsTerminal() {
					s.comm.Commit(nodeID, types.StatusTerminated, nil)
				}
			}
			return fmt.Errorf("wave deadline expired")
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
		}
	}
	return nil
}

// RunLoop executes the main MEK execution loop.
func (s *Scheduler) RunLoop(ctx context.Context, dispatch func(string) (*types.ExecutionResult, error)) (*types.MEKOutput, error) {
	metrics := &types.MEKMetrics{}

	for layerIdx := 0; layerIdx < len(s.Waves); layerIdx++ {
		waves := s.Waves[layerIdx]
		if len(waves) == 0 {
			// Check if we need to recompute (nodes may have transitioned)
			s.Recompute()
			continue
		}

		for waveIdx, wave := range waves {
			// Filter to only READY nodes that haven't been claimed yet
			ready, err := s.Claim(wave)
			if err != nil {
				return nil, fmt.Errorf("claim wave %d.%d: %w", layerIdx, waveIdx, err)
			}

			if len(ready) == 0 {
				// No READY nodes in this wave — recompute and continue
				s.Recompute()
				continue
			}

			// Determine wave deadline
			var waveDeadline time.Duration
			for _, nodeID := range wave {
				node := s.ceg.NodeMap[nodeID]
				if node != nil && node.Scheduling.Deadline != "" {
					d, _ := types.DeadlineDuration(node.Scheduling.Deadline)
					if d > waveDeadline {
						waveDeadline = d
					}
				}
			}

			// Dispatch concurrently
			var wg sync.WaitGroup
			results := make(chan struct {
				nodeID string
				result *types.ExecutionResult
				err    error
			}, len(ready))

			for _, nodeID := range ready {
				wg.Add(1)
				go func(nid string) {
					defer wg.Done()
					result, err := dispatch(nid)
					results <- struct {
						nodeID string
						result *types.ExecutionResult
						err    error
					}{nid, result, err}
				}(nodeID)
			}

			go func() {
				wg.Wait()
				close(results)
			}()

			// Collect and commit results
			for r := range results {
				if r.err != nil {
					s.comm.Commit(r.nodeID, types.StatusFailure, &types.ExecutionResult{
						Status: types.StatusFailure,
						Outputs: map[string]interface{}{
							"error": r.err.Error(),
						},
					})
					metrics.NodesFailed++
					continue
				}

			status := r.result.Status
			if r.result.Escalation {
				status = types.StatusTerminated
			}

			s.comm.Commit(r.nodeID, status, r.result)
				metrics.NodesExecuted++
				if status == types.StatusFailure {
					metrics.NodesFailed++
				}

				// M-013: ESCALATE propagates to system-level termination
				if r.result.Escalation {
					s.terminateAll()
					metrics.EscalationRequested = true

					output := &types.MEKOutput{
						StatusMap: s.sm.All(),
						Metrics:   *metrics,
					}
					return output, nil
				}
			}

			metrics.WavesCompleted++

			// Wait for all nodes in wave to be terminal (or deadline)
			if err := s.WaitWave(ctx, wave, waveDeadline); err != nil {
				// Deadline expired — terminate wave
				for _, nodeID := range wave {
					status, ok := s.sm.Get(nodeID)
					if ok && !status.IsTerminal() {
						s.comm.Commit(nodeID, types.StatusTerminated, nil)
					}
				}
			}
		}

		// After all waves in layer: recompute
		s.Recompute()
	}

	// Final recompute for any stragglers
	s.Recompute()

	return &types.MEKOutput{
		StatusMap: s.sm.All(),
		Metrics:   *metrics,
	}, nil
}

// terminateAll sets all non-terminal nodes to TERMINATED.
func (s *Scheduler) terminateAll() {
	for _, node := range s.ceg.Nodes {
		status, ok := s.sm.Get(node.ID)
		if ok && !status.IsTerminal() {
			s.comm.Commit(node.ID, types.StatusTerminated, nil)
		}
	}
}
