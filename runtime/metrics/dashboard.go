package metrics

import (
	"fmt"
	"strings"
)

type CLIDashboard struct {
	registry *MetricsRegistry
}

func NewCLIDashboard(reg *MetricsRegistry) *CLIDashboard {
	if reg == nil {
		reg = Global()
	}
	return &CLIDashboard{registry: reg}
}

func (d *CLIDashboard) Render() string {
	snap := d.registry.Snapshot()
	var b strings.Builder

	b.WriteString(header("Gateway Metrics Dashboard"))
	b.WriteString(fmt.Sprintf("Uptime: %ds | Throughput: %.1f req/s | Block Rate: %.1f%% | Error Rate: %.1f%%\n",
		snap.Runtime.UptimeSeconds, snap.Runtime.ThroughputRPS,
		snap.Runtime.BlockRate*100, snap.Runtime.ErrorRate*100))

	b.WriteString(header("Gateway Counters"))
	b.WriteString(fmt.Sprintf("Total: %d | Allowed: %d | Blocked: %d | Failed: %d | Rate Limited: %d | Panics: %d\n",
		snap.Gateway.RequestsTotal, snap.Gateway.RequestsAllowed,
		snap.Gateway.RequestsBlocked, snap.Gateway.RequestsFailed,
		snap.Gateway.RateLimited, snap.Gateway.Panics))

	b.WriteString(header("Enforcement"))
	b.WriteString(fmt.Sprintf("Evaluations: %d | Allowed: %d | Blocked: %d | Violations: %d\n",
		snap.Enforcement.Evaluations, snap.Enforcement.Allowed,
		snap.Enforcement.Blocked, snap.Enforcement.Violations))

	b.WriteString(header("Policy Intelligence"))
	b.WriteString(fmt.Sprintf("Events: %d | Drifts: %d | Suggestions: %d | Weight Updates: %d\n",
		snap.PolicyIntel.EventsRecorded, snap.PolicyIntel.DriftDetections,
		snap.PolicyIntel.Suggestions, snap.PolicyIntel.WeightUpdates))

	b.WriteString(header("Learning Engine"))
	b.WriteString(fmt.Sprintf("Updates: %d | Success: %d | Failure: %d | Avg Latency: %dms\n",
		snap.Learning.Updates, snap.Learning.SuccessCount,
		snap.Learning.FailureCount, snap.Learning.AvgLatencyMs))

	if len(snap.Stages) > 0 {
		b.WriteString(header("Per-Stage"))
		b.WriteString(fmt.Sprintf("%-20s %12s %12s %12s %10s\n", "Stage", "Invocations", "Avg (ns)", "Max (ns)", "Failures"))
		b.WriteString(divider())
		for _, s := range snap.Stages {
			b.WriteString(fmt.Sprintf("%-20s %12d %12d %12d %10d\n",
				s.Label, s.Invocations, s.AvgLatencyNs, s.MaxLatencyNs, s.Failures))
		}
	}

	return b.String()
}

func (d *CLIDashboard) RenderCompact() string {
	snap := d.registry.Snapshot()
	var b strings.Builder

	b.WriteString(fmt.Sprintf("[%ds] %.1f rps | req=%d ok=%d blk=%d fail=%d rate=%d enfc=%d blk=%d | pi=%d drf=%d sug=%d | learn=%d ok=%d fail=%d\n",
		snap.Runtime.UptimeSeconds,
		snap.Runtime.ThroughputRPS,
		snap.Gateway.RequestsTotal,
		snap.Gateway.RequestsAllowed,
		snap.Gateway.RequestsBlocked,
		snap.Gateway.RequestsFailed,
		snap.Gateway.RateLimited,
		snap.Enforcement.Evaluations,
		snap.Enforcement.Blocked,
		snap.PolicyIntel.EventsRecorded,
		snap.PolicyIntel.DriftDetections,
		snap.PolicyIntel.Suggestions,
		snap.Learning.Updates,
		snap.Learning.SuccessCount,
		snap.Learning.FailureCount,
	))

	return b.String()
}

func header(title string) string {
	const width = 60
	pad := (width - len(title) - 2) / 2
	if pad < 0 {
		pad = 0
	}
	line := strings.Repeat("─", width)
	return fmt.Sprintf("\n%s\n %s%s\n%s\n", line, strings.Repeat(" ", pad), title, line)
}

func divider() string {
	return strings.Repeat("─", 66) + "\n"
}
