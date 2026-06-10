package release

import (
	"crypto/sha256"
	"encoding/binary"
	"math"
	"time"
)

const (
	maxRetryAttempts    = 5
	baseBackoffSeconds  = 30.0
	maxBackoffSeconds   = 3600.0
	jitterRange float64 = 0.2
)

type RetryState struct {
	Attempts    int   `json:"attempts"`
	NextRetryAt int64 `json:"next_retry_at"`
	Finalized   bool  `json:"finalized"`
}

func ComputeNextRetry(attempts int, commitSHA, version string) time.Duration {
	if attempts <= 0 {
		return 0
	}
	if attempts > maxRetryAttempts {
		return 0
	}

	backoff := baseBackoffSeconds * math.Pow(2, float64(attempts-1))
	if backoff > maxBackoffSeconds {
		backoff = maxBackoffSeconds
	}

	jitter := deterministicJitter(commitSHA, version, attempts, jitterRange)
	total := backoff * (1 + jitter)

	return time.Duration(total * float64(time.Second))
}

func deterministicJitter(commitSHA, version string, attempt int, maxJitter float64) float64 {
	h := sha256.Sum256([]byte(commitSHA + version + string(rune(attempt))))
	seed := binary.BigEndian.Uint64(h[:8])
	return (float64(seed % 1000000) / 1000000.0) * maxJitter
}

func NewRetryState(commitSHA, version string) RetryState {
	delay := ComputeNextRetry(1, commitSHA, version)
	return RetryState{
		Attempts:    0,
		NextRetryAt: time.Now().Add(delay).UnixMilli(),
		Finalized:   false,
	}
}

func AdvanceRetryState(rs RetryState, commitSHA, version string) RetryState {
	rs.Attempts++
	if rs.Attempts >= maxRetryAttempts {
		rs.Finalized = true
		return rs
	}
	delay := ComputeNextRetry(rs.Attempts, commitSHA, version)
	rs.NextRetryAt = time.Now().Add(delay).UnixMilli()
	return rs
}

func IsRetryDue(rs RetryState) bool {
	return !rs.Finalized && time.Now().UnixMilli() >= rs.NextRetryAt
}
