package mcpv2

import (
	"log"
	"math"
	"sync"
	"time"
)

type ServerWeights struct {
	CapabilityFit float64
	KeywordMatch  float64
	History       float64
}

func (w ServerWeights) Factors() (capW, kwW, histW float64) {
	return w.CapabilityFit, w.KeywordMatch, w.History
}

type RoutingOutcome struct {
	RequestID      string
	SelectedServer string
	Success        bool
	LatencyMs      int64
	Timestamp      time.Time
}

type LearningEngine struct {
	mu      sync.RWMutex
	weights map[string]*ServerWeights
}

func NewLearningEngine() *LearningEngine {
	return &LearningEngine{
		weights: make(map[string]*ServerWeights),
	}
}

func (le *LearningEngine) WeightsFor(server string) ServerWeights {
	le.mu.RLock()
	defer le.mu.RUnlock()
	if w, ok := le.weights[server]; ok {
		return *w
	}
	return ServerWeights{CapabilityFit: 0.30, KeywordMatch: 0.40, History: 0.30}
}

type ExplorationState struct {
	mu              sync.RWMutex
	selectionCount  map[string]int
	totalSelections int
	ExplorationRate float64
}

func NewExplorationState(rate float64) *ExplorationState {
	return &ExplorationState{
		selectionCount:  make(map[string]int),
		ExplorationRate: rate,
	}
}

func (es *ExplorationState) RecordSelection(server string) {
	es.mu.Lock()
	defer es.mu.Unlock()
	es.selectionCount[server]++
	es.totalSelections++
}

func (es *ExplorationState) AdjustScore(server string, baseScore float64) float64 {
	return es.AdjustScoreWithRate(server, baseScore, es.ExplorationRate)
}

func (es *ExplorationState) AdjustScoreWithRate(server string, baseScore float64, rate float64) float64 {
	if rate <= 0 {
		return baseScore
	}
	es.mu.RLock()
	count := es.selectionCount[server]
	total := es.totalSelections
	es.mu.RUnlock()

	if total == 0 {
		return baseScore + rate
	}

	freq := float64(count) / float64(total)
	bonus := rate * (1.0 - freq)
	penalty := rate * freq

	return baseScore + bonus - penalty
}

func (le *LearningEngine) Update(outcome RoutingOutcome) {
	le.mu.Lock()
	defer le.mu.Unlock()

	w, ok := le.weights[outcome.SelectedServer]
	if !ok {
		w = &ServerWeights{CapabilityFit: 0.30, KeywordMatch: 0.40, History: 0.30}
		le.weights[outcome.SelectedServer] = w
	}

	rate := 0.05
	if outcome.Success {
		w.History += rate
	} else {
		w.History -= rate
		if w.History < 0.01 {
			w.History = 0.01
		}
	}

	log.Printf("[learning] server=%s success=%v latency=%d history=%.3f",
		outcome.SelectedServer, outcome.Success, outcome.LatencyMs, w.History)
}

type StabilityEngine struct {
	mu               sync.RWMutex
	decayRate        float64
	windowSize       int
	usageCount       map[string]int
	operationHistory map[string][]string
	convergenceScore map[string]float64
	oscillationCount map[string]int
	stabilityBias    map[string]float64
}

type StabilityMetrics struct {
	OscillationCount map[string]int
	ConvergenceScore map[string]float64
	StabilityIndex   float64
}

func NewStabilityEngine(decayRate float64, windowSize int) *StabilityEngine {
	return &StabilityEngine{
		decayRate:        decayRate,
		windowSize:       windowSize,
		usageCount:       make(map[string]int),
		operationHistory: make(map[string][]string),
		convergenceScore: make(map[string]float64),
		oscillationCount: make(map[string]int),
		stabilityBias:    make(map[string]float64),
	}
}

func (se *StabilityEngine) EffectiveRate(server string, baseRate float64) float64 {
	se.mu.RLock()
	count := se.usageCount[server]
	se.mu.RUnlock()
	rate := baseRate * math.Exp(-se.decayRate*float64(count))
	minRate := baseRate * 0.01
	if rate < minRate {
		return minRate
	}
	return rate
}

func (se *StabilityEngine) AdjustScore(server, operation string, score float64) float64 {
	se.mu.RLock()
	defer se.mu.RUnlock()
	oscPenalty := float64(se.oscillationCount[operation]) * 0.02
	bias := se.stabilityBias[server]
	return score - oscPenalty + bias
}

func (se *StabilityEngine) RecordSelection(operation, server string) {
	se.mu.Lock()
	defer se.mu.Unlock()

	se.usageCount[server]++

	hist := se.operationHistory[operation]
	if len(hist) >= se.windowSize {
		hist = hist[1:]
	}
	hist = append(hist, server)
	se.operationHistory[operation] = hist

	osc := detectOscillation(hist)
	se.oscillationCount[operation] = osc

	cvg := computeConvergence(hist)
	se.convergenceScore[operation] = cvg

	if cvg > 0.5 {
		se.stabilityBias[server] += 0.01
	}
}

func detectOscillation(history []string) int {
	if len(history) < 4 {
		return 0
	}
	count := 0
	for i := 2; i < len(history); i++ {
		if history[i] == history[i-2] && history[i] != history[i-1] {
			count++
		}
	}
	return count
}

func computeConvergence(history []string) float64 {
	if len(history) == 0 {
		return 0
	}
	counts := make(map[string]int)
	for _, s := range history {
		counts[s]++
	}
	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}
	return float64(maxCount) / float64(len(history))
}

func (se *StabilityEngine) ConvergenceScore(operation string) float64 {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.convergenceScore[operation]
}

func (se *StabilityEngine) OscillationCount(operation string) int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.oscillationCount[operation]
}

func (se *StabilityEngine) Metrics() StabilityMetrics {
	se.mu.RLock()
	defer se.mu.RUnlock()

	oscCopy := make(map[string]int, len(se.oscillationCount))
	for k, v := range se.oscillationCount {
		oscCopy[k] = v
	}
	cvgCopy := make(map[string]float64, len(se.convergenceScore))
	for k, v := range se.convergenceScore {
		cvgCopy[k] = v
	}

	totalOps := len(se.convergenceScore)
	var sum float64
	for _, c := range se.convergenceScore {
		sum += c
	}
	var idx float64
	if totalOps > 0 {
		idx = sum / float64(totalOps)
	}

	return StabilityMetrics{
		OscillationCount: oscCopy,
		ConvergenceScore: cvgCopy,
		StabilityIndex:   idx,
	}
}

func (se *StabilityEngine) UsageCount(server string) int {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.usageCount[server]
}

func (se *StabilityEngine) StabilityBias(server string) float64 {
	se.mu.RLock()
	defer se.mu.RUnlock()
	return se.stabilityBias[server]
}
