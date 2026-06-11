package mcpv2

import (
	"fmt"
	"log"
	"math"
	"sync"
)

type PolicyEvent struct {
	TraceID   string
	RequestID string
	Server    string
	Operation string
	Allowed   bool
	Blocked   bool
	Reason    string
}

type PolicySuggestion struct {
	Server          string  `json:"server"`
	Operation       string  `json:"operation"`
	SuggestedAction string  `json:"suggested_action"`
	Confidence      float64 `json:"confidence"`
}

type PolicyIntelligenceEngine struct {
	mu      sync.RWMutex
	History []PolicyEvent
	Weights map[string]float64
	Drifts  map[string]int
}

func NewPolicyIntelligenceEngine() *PolicyIntelligenceEngine {
	return &PolicyIntelligenceEngine{
		History: make([]PolicyEvent, 0),
		Weights: make(map[string]float64),
		Drifts:  make(map[string]int),
	}
}

func (pie *PolicyIntelligenceEngine) Record(event PolicyEvent) {
	pie.mu.Lock()
	defer pie.mu.Unlock()

	pie.History = append(pie.History, event)
	key := event.Server + ":" + event.Operation

	if event.Allowed {
		pie.Weights[key] += 0.01
	} else if event.Blocked {
		pie.Weights[key] -= 0.02
	}

	blockCount := 0
	for i := len(pie.History) - 1; i >= 0 && i >= len(pie.History)-10; i-- {
		e := pie.History[i]
		if e.Server == event.Server && e.Operation == event.Operation && e.Blocked {
			blockCount++
		}
	}
	if blockCount >= 3 {
		pie.Drifts[key] = blockCount
	}

	log.Printf("[policy_intel] recorded: server=%s op=%s allowed=%v blocked=%v weights=%s",
		event.Server, event.Operation, event.Allowed, event.Blocked, formatWeights(pie.Weights))
}

func formatWeights(w map[string]float64) string {
	if len(w) == 0 {
		return "{}"
	}
	first := true
	result := "{"
	for k, v := range w {
		if !first {
			result += " "
		}
		result += fmt.Sprintf("%s=%+.2f", k, v)
		first = false
	}
	return result + "}"
}

func (pie *PolicyIntelligenceEngine) DetectDrift(server, operation string) (bool, int) {
	pie.mu.RLock()
	defer pie.mu.RUnlock()
	key := server + ":" + operation
	count, ok := pie.Drifts[key]
	if !ok {
		return false, 0
	}
	return count >= 3, count
}

func (pie *PolicyIntelligenceEngine) GenerateSuggestions() []PolicySuggestion {
	pie.mu.RLock()
	defer pie.mu.RUnlock()

	suggestions := make([]PolicySuggestion, 0)

	for key, driftCount := range pie.Drifts {
		server, operation := splitKey(key)
		if server == "" || operation == "" {
			continue
		}
		weight := pie.Weights[key]
		negativeWeight := weight < -0.05
		totalEvents := 0
		for _, e := range pie.History {
			if e.Server == server && e.Operation == operation {
				totalEvents++
			}
		}

		if driftCount >= 3 && negativeWeight && totalEvents >= 5 {
			confidence := math.Min(0.5+float64(driftCount)*0.1, 0.95)
			suggestions = append(suggestions, PolicySuggestion{
				Server:          server,
				Operation:       operation,
				SuggestedAction: "review_policy",
				Confidence:      confidence,
			})
		}
	}

	return suggestions
}

func splitKey(key string) (string, string) {
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			return key[:i], key[i+1:]
		}
	}
	return "", ""
}

func (pie *PolicyIntelligenceEngine) EventCount() int {
	pie.mu.RLock()
	defer pie.mu.RUnlock()
	return len(pie.History)
}

func (pie *PolicyIntelligenceEngine) Weight(server, operation string) float64 {
	pie.mu.RLock()
	defer pie.mu.RUnlock()
	return pie.Weights[server+":"+operation]
}

func (pie *PolicyIntelligenceEngine) DriftCount(server, operation string) int {
	pie.mu.RLock()
	defer pie.mu.RUnlock()
	return pie.Drifts[server+":"+operation]
}
