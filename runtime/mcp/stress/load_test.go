package stress

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp"
)

type LoadResult struct {
	Scenario       string  `json:"scenario"`
	Concurrency    int     `json:"concurrency"`
	Requests       int     `json:"requests"`
	SuccessCount   int     `json:"success_count"`
	FailureCount   int     `json:"failure_count"`
	DeniedCount    int     `json:"denied_count"`
	DurationMs     int64   `json:"duration_ms"`
	ThroughputRPS  float64 `json:"throughput_rps"`
}

func runLoadTest(t *testing.T, concurrency, totalRequests int, tool string) LoadResult {
	ke := setupKernel(t)
	defer ke.GracefulShutdown()

	ig := mcp.NewIntegrationGateway(ke)

	var wg sync.WaitGroup
	var success, fail atomic.Int64

	start := time.Now()

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for j := 0; j < totalRequests/concurrency; j++ {
				resp := ig.Process(mapToJSON(fmt.Sprintf("ld_%d_%d", workerID, j),
					fmt.Sprintf("sess_%d", workerID%10),
					"p1", tool, "read", map[string]any{"path": "/home/asem/workspace/VERSION"}))
				if resp.Success {
					success.Add(1)
				} else {
					fail.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()

	elapsed := time.Since(start)
	rps := float64(totalRequests) / elapsed.Seconds()

	return LoadResult{
		Scenario:      fmt.Sprintf("%d sessions, %d req", concurrency, totalRequests),
		Concurrency:   concurrency,
		Requests:      totalRequests,
		SuccessCount:  int(success.Load()),
		FailureCount:  int(fail.Load()),
		DurationMs:    elapsed.Milliseconds(),
		ThroughputRPS: rps,
	}
}

func TestConcurrentSessions(t *testing.T) {
	scenarios := []struct {
		name        string
		concurrency int
		total       int
	}{
		{"10 sessions / 50 req", 10, 50},
		{"20 sessions / 100 req", 20, 100},
		{"50 sessions / 200 req", 50, 200},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			result := runLoadTest(t, sc.concurrency, sc.total, "filesystem")
			t.Logf("[%s] success=%d fail=%d rps=%.1f duration=%dms",
				sc.name, result.SuccessCount, result.FailureCount,
				result.ThroughputRPS, result.DurationMs)

			if result.SuccessCount < sc.total/2 {
				t.Errorf("success rate too low: %d/%d", result.SuccessCount, sc.total)
			}
		})
	}
}

func TestRoutingSaturation(t *testing.T) {
	ke := setupKernel(t)
	defer ke.GracefulShutdown()

	ig := mcp.NewIntegrationGateway(ke)

	tools := []string{"filesystem", "git", "filesystem", "git", "filesystem"}
	start := time.Now()
	success := 0

	for i := 0; i < 100; i++ {
		tool := tools[i%len(tools)]
		resp := ig.Process(mapToJSON(fmt.Sprintf("sat_%d", i), "s1", "p1",
			tool, "read", map[string]any{"path": "/home/asem/workspace/VERSION"}))
		if resp.Success {
			success++
		}
	}

	t.Logf("[Routing Saturation] 100 req, success=%d, duration=%dms",
		success, time.Since(start).Milliseconds())

	if success < 50 {
		t.Errorf("routing success too low: %d/100", success)
	}
}

func TestToolContention(t *testing.T) {
	ke := setupKernel(t)
	defer ke.GracefulShutdown()
	ig := mcp.NewIntegrationGateway(ke)

	var wg sync.WaitGroup
	results := make(chan bool, 100)

	start := time.Now()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			resp := ig.Process(mapToJSON(fmt.Sprintf("tc_%d", id),
				"s1", "p1", "filesystem", "read",
				map[string]any{"path": "/home/asem/workspace/VERSION"}))
			results <- resp.Success
		}(i)
	}
	wg.Wait()
	close(results)

	success := 0
	for s := range results {
		if s {
			success++
		}
	}

	t.Logf("[Tool Contention] 50 concurrent on filesystem, success=%d, duration=%dms",
		success, time.Since(start).Milliseconds())

	if success < 25 {
		t.Errorf("contention success too low: %d/50", success)
	}
}
