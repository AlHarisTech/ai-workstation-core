package replay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anomalyco/mek/internal/journal"
	"github.com/anomalyco/mek/internal/runtime"
	"github.com/anomalyco/mek/pkg/types"
)

// Divergence records a mismatch between the replayed execution and the journal.
type Divergence struct {
	NodeID       string           `json:"node_id"`
	JournalStatus types.NodeStatus `json:"journal_status"`
	ReplayStatus types.NodeStatus `json:"replay_status"`
}

// Report is the output of a replay verification.
type Report struct {
	RIRPath     string       `json:"rir_path"`
	JournalPath string       `json:"journal_path"`
	Match       bool         `json:"match"`
	Divergences []Divergence `json:"divergences,omitempty"`
	JournalEntries int       `json:"journal_entries"`
	ReplayNodes    int       `json:"replay_nodes"`
}

// Verify re-executes the RIR and compares the resulting terminal states
// against the journal's recorded terminal states.
// Returns a Report indicating whether the replay matched the original execution.
func Verify(ctx context.Context, rirPath, journalPath string) (*Report, error) {
	report := &Report{
		RIRPath:     rirPath,
		JournalPath: journalPath,
	}

	// Load journal terminal states
	journalTerminals, err := loadJournalTerminals(journalPath)
	if err != nil {
		return nil, fmt.Errorf("replay: load journal: %w", err)
	}
	report.JournalEntries = len(journalTerminals)

	// Re-execute MEK with the same RIR
	mek, err := runtime.New(rirPath, nil)
	if err != nil {
		return nil, fmt.Errorf("replay: create MEK: %w", err)
	}

	output, err := mek.Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("replay: execute MEK: %w", err)
	}

	report.ReplayNodes = len(output.StatusMap)

	// Compare terminal states
	report.Match = true
	for nodeID, journalStatus := range journalTerminals {
		replayState, ok := output.StatusMap[nodeID]
		if !ok {
			report.Divergences = append(report.Divergences, Divergence{
				NodeID:        nodeID,
				JournalStatus: journalStatus,
				ReplayStatus:  "MISSING",
			})
			report.Match = false
			continue
		}
		if replayState.Status != journalStatus {
			report.Divergences = append(report.Divergences, Divergence{
				NodeID:        nodeID,
				JournalStatus: journalStatus,
				ReplayStatus:  replayState.Status,
			})
			report.Match = false
		}
	}

	// Check for nodes in replay but not in journal
	for nodeID, state := range output.StatusMap {
		if _, ok := journalTerminals[nodeID]; !ok {
			report.Divergences = append(report.Divergences, Divergence{
				NodeID:        nodeID,
				JournalStatus: "MISSING",
				ReplayStatus:  state.Status,
			})
			report.Match = false
		}
	}

	return report, nil
}

func loadJournalTerminals(path string) (map[string]types.NodeStatus, error) {
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
			continue // skip malformed lines
		}
		// Last write wins — the final entry for each node is its terminal status
		terminals[entry.NodeID] = entry.ToStatus
	}
	return terminals, scanner.Err()
}
