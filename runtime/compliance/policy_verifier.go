package compliance

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/kernel"
	"github.com/AlHarisTech/ai-workstation-core/runtime/types"
)

func VerifyPolicy(t *testing.T) PolicyComplianceReport {
	t.Helper()
	report := PolicyComplianceReport{
		Timestamp:       time.Now(),
		CompliancePass:  true,
	}

	cfg := kernel.DefaultConfig(); ws := tempWorkspaceRoot(); defer os.RemoveAll(ws); cfg.WorkspaceRoot = ws
	cfg.WorkerCount = 2
	harden := kernel.DefaultHardeningConfig()

	engine, err := kernel.NewKernelEngine(cfg, harden)
	if err != nil {
		t.Fatalf("engine init: %v", err)
	}
	engine.Start()

	tests := []struct {
		name        string
		sessionID   string
		projectID   string
		tool        string
		path        string
		expectDeny  bool
	}{
		{"invalid_session", "", "", "echo", "", true},
		{"missing_project", "s1", "", "echo", "", true},
		{"denied_path", "s1", "p1", "filesystem_read", "/etc/passwd", true},
		{"unknown_tool", "s1", "p1", "no_such_tool", "", true},
		{"invalid_request", "s1", "p1", "echo", "", false},
	}

	for _, tc := range tests {
		input := map[string]interface{}{
			"id":     fmt.Sprintf("policy_%s", tc.name),
			"method": "tool.call",
			"params": map[string]interface{}{
				"tool":      tc.tool,
				"arguments": map[string]interface{}{},
			},
			"session": map[string]interface{}{
				"session_id": tc.sessionID,
				"project_id": tc.projectID,
			},
		}
		if tc.path != "" {
			input["params"].(map[string]interface{})["arguments"] = map[string]interface{}{"path": tc.path}
		}

		data, _ := json.Marshal(input)
		err := engine.Ingest(data)
		wasDenied := err != nil

		result := PolicyTestResult{
			TestName:     tc.name,
			ExpectedDeny: tc.expectDeny,
			WasDenied:    wasDenied,
		}

		if tc.expectDeny && !wasDenied {
			result.Bypassed = true
			report.BypassDetected++
			report.CompliancePass = false
		}

		if !tc.expectDeny && wasDenied {
			result.Bypassed = true
		}

		report.Details = append(report.Details, result)
		report.TestsExecuted++

		if wasDenied {
			report.DenyEvents++
		}
	}

	time.Sleep(200 * time.Millisecond)

	_ = types.PipelineMode("")
	engine.GracefulShutdown()
	return report
}
