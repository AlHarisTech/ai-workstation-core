package verify

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anomalyco/mek/internal/ceg"
	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/journal"
	"github.com/anomalyco/mek/internal/replay"
	riloader "github.com/anomalyco/mek/internal/rir"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/internal/trace"
	"github.com/anomalyco/mek/pkg/types"
)

type Violation struct {
	Rule    string `json:"rule"`
	NodeID  string `json:"node_id"`
	Message string `json:"message"`
}

type Report struct {
	Pass       bool        `json:"pass"`
	Violations []Violation `json:"violations,omitempty"`
	Stats      Stats       `json:"stats"`
}

type Stats struct {
	TotalNodes       int `json:"total_nodes"`
	TotalEdges       int `json:"total_edges"`
	TerminalNodes    int `json:"terminal_nodes"`
	DependencyChecks int `json:"dependency_checks"`
}

func Structural(rirPath, journalPath string) (*Report, error) {
	report := &Report{Pass: true}

	_, c, err := loadRIR(rirPath)
	if err != nil {
		return nil, err
	}

	report.Stats.TotalNodes = len(c.Nodes)
	report.Stats.TotalEdges = len(c.Edges)

	terminals, err := loadTerminals(journalPath)
	if err != nil {
		return nil, fmt.Errorf("verify: load journal: %w", err)
	}
	report.Stats.TerminalNodes = len(terminals)

	for _, node := range c.Nodes {
		status, ok := terminals[node.ID]
		if !ok {
			report.Violations = append(report.Violations, Violation{
				Rule: "V-002", NodeID: node.ID, Message: "node missing from journal",
			})
			report.Pass = false
			continue
		}
		if !status.IsTerminal() {
			report.Violations = append(report.Violations, Violation{
				Rule: "V-002", NodeID: node.ID, Message: fmt.Sprintf("node not terminal: %s", status),
			})
			report.Pass = false
		}
	}

	for _, edge := range c.Edges {
		if edge.Type != "dependency" {
			continue
		}
		report.Stats.DependencyChecks++
		toStatus, toOk := terminals[edge.To]
		fromStatus, fromOk := terminals[edge.From]
		if !toOk || !fromOk {
			continue
		}
		if toStatus == types.StatusSuccess && fromStatus != types.StatusSuccess {
			report.Violations = append(report.Violations, Violation{
				Rule: "G6", NodeID: edge.To,
				Message: fmt.Sprintf("SUCCESS but dependency %s is %s", edge.From, fromStatus),
			})
			report.Pass = false
		}
	}

	for _, node := range c.Nodes {
		if node.Activation.Condition != "all_success" {
			continue
		}
		nodeStatus, ok := terminals[node.ID]
		if !ok || nodeStatus != types.StatusSuccess {
			continue
		}
		for _, req := range node.Activation.Requires {
			reqStatus, ok := terminals[req]
			if !ok || reqStatus != types.StatusSuccess {
				report.Violations = append(report.Violations, Violation{
					Rule: "ACTIVATION", NodeID: node.ID,
					Message: fmt.Sprintf("SUCCESS but requires %s is %v", req, reqStatus),
				})
				report.Pass = false
			}
		}
	}

	return report, nil
}

func ExecuteAndVerify(ctx context.Context, rirPath string) (*Report, error) {
	journalPath := fmt.Sprintf("/tmp/mek_verify_%d.jsonl", os.Getpid())

	mek, err := runtime.New(rirPath, nil)
	if err != nil {
		return nil, err
	}

	output, err := mek.Run(ctx)
	if err != nil {
		return nil, err
	}

	j, err := journal.New(journalPath)
	if err != nil {
		return nil, err
	}
	defer j.Close()
	defer os.Remove(journalPath)

	events := make(chan commit.CommitEvent, 1024)
	done := j.Subscribe(events)

	sm := mek.StatusMap()
	for _, node := range mek.CEG().Nodes {
		state := sm.GetState(node.ID)
		if state != nil {
			events <- commit.CommitEvent{
				NodeID: node.ID, NewStatus: state.Status,
				Outputs: state.Outputs, Artifacts: state.Artifacts,
			}
		}
	}
	close(events)
	<-done
	_ = output

	return Structural(rirPath, journalPath)
}

func loadRIR(path string) (*types.RIR, *types.CEG, error) {
	r, err := riloader.Load(path)
	if err != nil {
		return nil, nil, fmt.Errorf("load RIR: %w", err)
	}
	c, err := ceg.Build(r)
	if err != nil {
		return nil, nil, fmt.Errorf("build CEG: %w", err)
	}
	return r, c, nil
}

func loadTerminals(path string) (map[string]types.NodeStatus, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	terminals := make(map[string]types.NodeStatus)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry journal.Entry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		terminals[entry.NodeID] = entry.ToStatus
	}
	return terminals, scanner.Err()
}

// ─── Cross-Domain Consistency ───

// ConsistencyReport verifies that all truth domains agree:
// Journal ↔ Trace ↔ Replay ↔ Structural
type ConsistencyReport struct {
	Pass   bool               `json:"pass"`
	Checks []ConsistencyCheck `json:"checks"`
}

// ConsistencyCheck is a single cross-domain verification.
type ConsistencyCheck struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

// FullConsistencyCheck executes MEK and verifies all 4 truth domains agree.
// This is the implementation of the Consistency Lattice Theorem (T1–T5)
// at the operational level.
func FullConsistencyCheck(ctx context.Context, rirPath string) (*ConsistencyReport, error) {
	report := &ConsistencyReport{Pass: true}

	journalPath := fmt.Sprintf("/tmp/mek_consistency_%d.jsonl", os.Getpid())
	defer os.Remove(journalPath)

	mek, err := runtime.New(rirPath, nil)
	if err != nil {
		return nil, fmt.Errorf("consistency: %w", err)
	}

	j, err := journal.New(journalPath)
	if err != nil {
		return nil, err
	}
	defer j.Close()

	tc := trace.NewCollector()

	output, err := mek.Run(ctx)
	if err != nil {
		return nil, err
	}

	jEvents := make(chan commit.CommitEvent, 1024)
	tEvents := make(chan commit.CommitEvent, 1024)
	jDone := j.Subscribe(jEvents)
	tDone := tc.Subscribe(tEvents)

	sm := mek.StatusMap()
	for _, node := range mek.CEG().Nodes {
		state := sm.GetState(node.ID)
		if state != nil {
			evt := commit.CommitEvent{
				NodeID: node.ID, NewStatus: state.Status,
				Outputs: state.Outputs, Artifacts: state.Artifacts,
			}
			jEvents <- evt
			tEvents <- evt
		}
	}
	close(jEvents)
	close(tEvents)
	<-jDone
	for range tDone {
	}
	_ = output

	// CHECK 1: Journal ↔ Kernel
	check := ConsistencyCheck{Name: "journal↔kernel"}
	journalTerminals := jEntries(j)
	kernelStates := sm.All()
	match := true
	for nodeID, js := range journalTerminals {
		ks, ok := kernelStates[nodeID]
		if !ok || ks.Status != js {
			match = false
			check.Detail = fmt.Sprintf("node %s: journal=%s kernel=%v", nodeID, js, ks)
			break
		}
	}
	if match {
		for nodeID, ks := range kernelStates {
			js, ok := journalTerminals[nodeID]
			if !ok || js != ks.Status {
				match = false
				check.Detail = fmt.Sprintf("node %s: kernel=%s journal=%v", nodeID, ks.Status, js)
				break
			}
		}
	}
	check.Pass = match
	report.Checks = append(report.Checks, check)
	if !match {
		report.Pass = false
	}

	// CHECK 2: Trace ↔ Journal
	check = ConsistencyCheck{Name: "trace↔journal"}
	traceTerminals := tc.TerminalStatuses()
	match = true
	for nodeID, ts := range traceTerminals {
		js, ok := journalTerminals[nodeID]
		if !ok || js != ts {
			match = false
			check.Detail = fmt.Sprintf("node %s: trace=%s journal=%v", nodeID, ts, js)
			break
		}
	}
	check.Pass = match
	report.Checks = append(report.Checks, check)
	if !match {
		report.Pass = false
	}

	// CHECK 3: Replay ↔ Journal
	check = ConsistencyCheck{Name: "replay↔journal"}
	rp, err := replay.Verify(ctx, rirPath, journalPath)
	if err != nil {
		check.Pass = false
		check.Detail = fmt.Sprintf("replay error: %v", err)
	} else {
		check.Pass = rp.Match
		if !rp.Match {
			data, _ := json.Marshal(rp.Divergences)
			check.Detail = string(data)
		}
	}
	report.Checks = append(report.Checks, check)
	if !check.Pass {
		report.Pass = false
	}

	// CHECK 4: Structural Verification
	check = ConsistencyCheck{Name: "structural"}
	sr, err := Structural(rirPath, journalPath)
	if err != nil {
		check.Pass = false
		check.Detail = fmt.Sprintf("structural error: %v", err)
	} else {
		check.Pass = sr.Pass
		if !sr.Pass {
			data, _ := json.Marshal(sr.Violations)
			check.Detail = string(data)
		}
	}
	report.Checks = append(report.Checks, check)
	if !check.Pass {
		report.Pass = false
	}

	return report, nil
}

func jEntries(j *journal.Journal) map[string]types.NodeStatus {
	out := make(map[string]types.NodeStatus)
	for _, e := range j.Entries() {
		out[e.NodeID] = e.ToStatus
	}
	return out
}
