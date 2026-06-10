package contracts

const (
	WeightCI              = 25
	WeightRace            = 25
	WeightStress          = 25
	WeightFailureInjection = 25
	MaxReadinessScore     = 100
)

const (
	StatusReleaseReady      = "RELEASE_READY"
	StatusConditionallyReady = "CONDITIONALLY_READY"
	StatusPreRelease        = "PRE_RELEASE"
	StatusNotReady          = "NOT_READY"
)

func ReadinessLevel(score int) string {
	switch {
	case score >= 95:
		return StatusReleaseReady
	case score >= 85:
		return StatusConditionallyReady
	case score >= 70:
		return StatusPreRelease
	default:
		return StatusNotReady
	}
}

var StressScenarios = []struct {
	Name     string
	Sessions int
	Requests int
}{
	{"A_Light", 10, 100},
	{"B_Medium", 100, 1000},
	{"C_Heavy", 100, 5000},
	{"D_Saturation", 200, 10000},
}

var FailureScenarios = []struct {
	Name             string
	ExpectedBehavior string
}{
	{"WorkerPanic", "recover() catches panic, worker continues, request marked FAILED_EXECUTION"},
	{"QueueOverflow", "requestChan full → QUEUE_FULL rejection immediately"},
	{"StateWriteFailure", "retry once, then STATE_WRITE_FAILED, kernel continues"},
	{"ExecutionTimeout", "context deadline → EXECUTION_TIMEOUT envelope"},
	{"PolicyDenial", "DENY → skip execution, emit audit log, no side effects"},
	{"ReplayCorruption", "altered trace → hash mismatch → replay verification fails"},
}
