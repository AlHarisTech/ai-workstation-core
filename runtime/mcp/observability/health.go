package observability

import "math"

type HealthScores struct {
	ToolHealthScore     float64 `json:"tool_health_score"`
	RouterStabilityScore float64 `json:"router_stability_score"`
	GatewayLoadScore     float64 `json:"gateway_load_score"`
	SystemSaturationIndex float64 `json:"system_saturation_index"`
	OverallHealth       float64 `json:"overall_health"`
}

func ComputeHealthScores(snap TelemetrySnapshot, activePct float64, cbOpenCount int, cbTotalCount int) HealthScores {
	rejectRate := snap.RejectRate
	if rejectRate == 0 && snap.RequestTotal > 0 {
		rejectRate = float64(snap.RejectTotal) / float64(snap.RequestTotal)
	}

	toolHealth := computeToolHealth(cbOpenCount, cbTotalCount)
	routerStability := computeRouterStability(rejectRate)
	gatewayLoad := computeGatewayLoad(activePct)
	sysSat := computeSystemSaturation(activePct, rejectRate, cbOpenCount, cbTotalCount)

	overall := (toolHealth + routerStability + gatewayLoad + (1.0 - sysSat)) / 4.0
	overall = clamp(overall)

	return HealthScores{
		ToolHealthScore:      toolHealth,
		RouterStabilityScore: routerStability,
		GatewayLoadScore:     gatewayLoad,
		SystemSaturationIndex: sysSat,
		OverallHealth:        overall,
	}
}

func computeToolHealth(openCount, totalCount int) float64 {
	if totalCount == 0 {
		return 1.0
	}
	ratio := float64(openCount) / float64(totalCount)
	return clamp(1.0 - ratio)
}

func computeRouterStability(rejectRate float64) float64 {
	if rejectRate >= 0.5 {
		return 0.2
	}
	if rejectRate >= 0.3 {
		return 0.5
	}
	if rejectRate >= 0.1 {
		return 0.8
	}
	return 1.0
}

func computeGatewayLoad(activePct float64) float64 {
	return clamp(1.0 - activePct)
}

func computeSystemSaturation(activePct float64, rejectRate float64, openCount, totalCount int) float64 {
	loadFactor := activePct
	rejectFactor := math.Min(rejectRate*2, 1.0)

	cbFactor := 0.0
	if totalCount > 0 {
		cbFactor = math.Min(float64(openCount)/float64(totalCount), 1.0)
	}

	return clamp((loadFactor + rejectFactor + cbFactor) / 3.0)
}

func clamp(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
