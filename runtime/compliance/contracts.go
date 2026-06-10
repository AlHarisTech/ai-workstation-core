package compliance

const (
	WeightFairness  = 20
	WeightReplay    = 25
	WeightSLA       = 20
	WeightPolicy    = 20
	WeightShutdown  = 15
	MaxScore        = 100

	LevelCertified = "Production Certified"
	LevelReady     = "Production Ready"
	LevelLimited   = "Limited Production"
	LevelNon       = "Non-Compliant"
)

type SLAThresholds struct {
	P50Max float64
	P95Max float64
	P99Max float64
}

var DefaultSLA = SLAThresholds{
	P50Max: 10,
	P95Max: 50,
	P99Max: 120,
}

func PassBool(pass bool) ComplianceStatus {
	if pass {
		return Pass
	}
	return Fail
}

func CertificationLevel(score int) string {
	switch {
	case score >= 95:
		return LevelCertified
	case score >= 85:
		return LevelReady
	case score >= 70:
		return LevelLimited
	default:
		return LevelNon
	}
}
