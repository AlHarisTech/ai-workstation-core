package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/anomalyco/mek/internal/ceg"
	"github.com/anomalyco/mek/internal/commit"
	"github.com/anomalyco/mek/internal/dispatcher"
	"github.com/anomalyco/mek/internal/rir"
	"github.com/anomalyco/mek/internal/scheduler"
	"github.com/anomalyco/mek/pkg/types"
)

type MEK struct {
	rir      *types.RIR
	ceg      *types.CEG
	sm       *types.StatusMap
	comm     *commit.Engine
	sched    *scheduler.Scheduler
	disp     *dispatcher.Dispatcher
	cfg      *dispatcher.AdapterConfig
}

func New(rirPath string, cfg *dispatcher.AdapterConfig) (*MEK, error) {
	if cfg == nil {
		cfg = &dispatcher.AdapterConfig{
			AgentAdapters: make(map[string]dispatcher.AgentAdapterConfig),
			ToolAdapters:  make(map[string]dispatcher.ToolAdapterConfig),
			Providers:     make(map[string]dispatcher.ProviderConfig),
		}
	}

	// Load RIR
	r, err := rir.Load(rirPath)
	if err != nil {
		return nil, fmt.Errorf("load RIR: %w", err)
	}

	// Build CEG
	c, err := ceg.Build(r)
	if err != nil {
		return nil, fmt.Errorf("build CEG: %w", err)
	}

	// Initialize state
	sm := types.NewStatusMap()
	comm := commit.New(sm)
	sched := scheduler.New(c, sm, comm)
	disp := dispatcher.New(c, cfg)

	// Compute wave partition
	if err := sched.ComputeWaves(r); err != nil {
		return nil, fmt.Errorf("compute waves: %w", err)
	}

	// Initialize status map
	sched.InitStatusMap()

	return &MEK{
		rir:   r,
		ceg:   c,
		sm:    sm,
		comm:  comm,
		sched: sched,
		disp:  disp,
		cfg:   cfg,
	}, nil
}

func (m *MEK) Run(ctx context.Context) (*types.MEKOutput, error) {
	start := time.Now()

	output, err := m.sched.RunLoop(ctx, m.disp.Dispatch)
	if err != nil {
		return nil, fmt.Errorf("run loop: %w", err)
	}

	output.Metrics.TotalDurationMs = int(time.Since(start).Milliseconds())
	return output, nil
}

func (m *MEK) StatusMap() *types.StatusMap {
	return m.sm
}

func (m *MEK) CEG() *types.CEG {
	return m.ceg
}

func (m *MEK) RIR() *types.RIR {
	return m.rir
}

func Run(rirPath string, cfg *dispatcher.AdapterConfig) error {
	ctx := context.Background()

	mek, err := New(rirPath, cfg)
	if err != nil {
		return err
	}

	output, err := mek.Run(ctx)
	if err != nil {
		return err
	}

	// Print output as JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}
