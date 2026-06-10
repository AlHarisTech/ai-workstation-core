package observability

type SignalType string

const (
	SignalSaturationWarning    SignalType = "saturation_warning"
	SignalRoutingPressureHigh  SignalType = "routing_pressure_high"
	SignalRetryBudgetDepletion SignalType = "retry_budget_depletion_risk"
	SignalToolHealthDegradation SignalType = "tool_health_degradation"
)

type ControlSignal struct {
	Type      SignalType `json:"type"`
	Severity  string     `json:"severity"`
	Component string     `json:"component"`
	Message   string     `json:"message"`
	Value     float64    `json:"value"`
	Threshold float64    `json:"threshold"`
}

type ControlSignalState struct {
	SaturationWarning    bool `json:"saturation_warning"`
	RoutingPressureHigh  bool `json:"routing_pressure_high"`
	RetryBudgetDepletion bool `json:"retry_budget_depletion"`
	ToolHealthDegradation bool `json:"tool_health_degradation"`
}

const (
	saturationThreshold    = 0.8
	rejectRateThreshold    = 0.3
	retryDepletionThreshold = 0.5
)

func ComputeControlSignals(snap TelemetrySnapshot, activePct float64, cbOpenTools int, retryUsedPct float64) []ControlSignal {
	signals := make([]ControlSignal, 0)

	rejectRate := snap.RejectRate
	if rejectRate == 0 && snap.RequestTotal > 0 {
		rejectRate = float64(snap.RejectTotal) / float64(snap.RequestTotal)
	}

	if activePct >= saturationThreshold {
		signals = append(signals, ControlSignal{
			Type:      SignalSaturationWarning,
			Severity:  severityLevel(activePct),
			Component: "gateway",
			Message:   "backpressure near capacity",
			Value:     activePct,
			Threshold: saturationThreshold,
		})
	}

	if rejectRate >= rejectRateThreshold {
		signals = append(signals, ControlSignal{
			Type:      SignalRoutingPressureHigh,
			Severity:  severityLevel(snap.RejectRate),
			Component: "router",
			Message:   "routing rejection rate elevated",
			Value:     rejectRate,
			Threshold: rejectRateThreshold,
		})
	}

	if retryUsedPct >= retryDepletionThreshold {
		signals = append(signals, ControlSignal{
			Type:      SignalRetryBudgetDepletion,
			Severity:  severityLevel(retryUsedPct),
			Component: "retry",
			Message:   "retry budget consumption high",
			Value:     retryUsedPct,
			Threshold: retryDepletionThreshold,
		})
	}

	if cbOpenTools > 0 {
		signals = append(signals, ControlSignal{
			Type:      SignalToolHealthDegradation,
			Severity:  "warning",
			Component: "circuit_breaker",
			Message:   "tools in degraded state",
			Value:     float64(cbOpenTools),
			Threshold: 0,
		})
	}

	return signals
}

func severityLevel(pct float64) string {
	if pct >= 0.95 {
		return "critical"
	}
	if pct >= 0.85 {
		return "warning"
	}
	return "info"
}
