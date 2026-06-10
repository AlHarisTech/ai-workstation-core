package contracts

import (
	"fmt"
	"sync"
)

type ToolThrottleConfig struct {
	MaxConcurrent int
}

type BackpressureConfig struct {
	MaxQueuePerSession int
	MaxQueueTotal      int
	SoftRejectionPct   float64
	ToolThrottle       map[string]ToolThrottleConfig
}

func DefaultBackpressureConfig() BackpressureConfig {
	return BackpressureConfig{
		MaxQueuePerSession: 5,
		MaxQueueTotal:      1000,
		SoftRejectionPct:   0.8,
		ToolThrottle:       map[string]ToolThrottleConfig{},
	}
}

type BackpressureModel struct {
	mu           sync.Mutex
	sessionCount map[string]int
	toolCount    map[string]int
	totalActive  int
	config       BackpressureConfig
}

func NewBackpressureModel(cfg BackpressureConfig) *BackpressureModel {
	if cfg.MaxQueuePerSession <= 0 {
		cfg.MaxQueuePerSession = 5
	}
	if cfg.MaxQueueTotal <= 0 {
		cfg.MaxQueueTotal = 1000
	}
	if cfg.SoftRejectionPct <= 0 || cfg.SoftRejectionPct > 1 {
		cfg.SoftRejectionPct = 0.8
	}
	if cfg.ToolThrottle == nil {
		cfg.ToolThrottle = map[string]ToolThrottleConfig{}
	}
	return &BackpressureModel{
		sessionCount: make(map[string]int),
		toolCount:    make(map[string]int),
		config:       cfg,
	}
}

func (b *BackpressureModel) Acquire(sessionID, tool string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.totalActive >= b.config.MaxQueueTotal {
		return fmt.Errorf("BACKPRESSURE_REJECTED: system saturated (%d/%d)", b.totalActive, b.config.MaxQueueTotal)
	}

	if b.sessionCount[sessionID] >= b.config.MaxQueuePerSession {
		return fmt.Errorf("BACKPRESSURE_REJECTED: session %s at limit (%d/%d)", sessionID, b.sessionCount[sessionID], b.config.MaxQueuePerSession)
	}

	if c, ok := b.config.ToolThrottle[tool]; ok && b.toolCount[tool] >= c.MaxConcurrent {
		return fmt.Errorf("BACKPRESSURE_REJECTED: tool %s saturated (%d/%d)", tool, b.toolCount[tool], c.MaxConcurrent)
	}

	b.sessionCount[sessionID]++
	b.toolCount[tool]++
	b.totalActive++
	return nil
}

func (b *BackpressureModel) Release(sessionID, tool string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.sessionCount[sessionID] > 0 {
		b.sessionCount[sessionID]--
	}
	if b.toolCount[tool] > 0 {
		b.toolCount[tool]--
	}
	if b.totalActive > 0 {
		b.totalActive--
	}
}

func (b *BackpressureModel) IsSaturated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.totalActive >= int(float64(b.config.MaxQueueTotal)*b.config.SoftRejectionPct)
}

func (b *BackpressureModel) IsSoftRejected(sessionID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionCount[sessionID] >= b.config.MaxQueuePerSession
}

func (b *BackpressureModel) ActiveCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.totalActive
}

func (b *BackpressureModel) SessionCount(sessionID string) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.sessionCount[sessionID]
}

func (b *BackpressureModel) Config() BackpressureConfig {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.config
}
