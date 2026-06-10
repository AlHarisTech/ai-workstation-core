package hardening

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type TraceCompressor struct{}

type CompressedTrace struct {
	RequestID string        `json:"r"`
	Tool      string        `json:"t"`
	Action    string        `json:"a"`
	Success   bool          `json:"s"`
	LatencyMS int64         `json:"l"`
	Decision  string        `json:"d"`
	DeniedAt  string        `json:"da,omitempty"`
	PolicyIDs []string      `json:"p,omitempty"`
	Error     string        `json:"e,omitempty"`
	Timestamp int64         `json:"ts"`
}

type KernelNoiseFilter struct {
	skipStages map[string]bool
}

func NewKernelNoiseFilter() *KernelNoiseFilter {
	return &KernelNoiseFilter{
		skipStages: map[string]bool{
			"pre_validation":   true,
			"session_guard":    true,
			"post_validation":  true,
			"capability_routing": true,
		},
	}
}

func (knf *KernelNoiseFilter) FilterTrace(raw json.RawMessage) json.RawMessage {
	var trace map[string]interface{}
	if err := json.Unmarshal(raw, &trace); err != nil {
		return raw
	}

	stages, ok := trace["execution_trace"].([]interface{})
	if !ok {
		return raw
	}

	filtered := make([]interface{}, 0)
	for _, stage := range stages {
		s, ok := stage.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := s["stage"].(string)
		if knf.skipStages[name] {
			continue
		}
		filtered = append(filtered, s)
	}

	trace["execution_trace"] = filtered
	data, _ := json.Marshal(trace)
	return data
}

func CompressResponse(resp types.MCPResponse, decision string, deniedAt string, policyIDs []string) CompressedTrace {
	return CompressedTrace{
		RequestID: resp.ID,
		Tool:      "",
		Action:    "",
		Success:   resp.Success,
		LatencyMS: resp.LatencyMS,
		Decision:  decision,
		DeniedAt:  deniedAt,
		PolicyIDs: policyIDs,
		Error:     resp.Error,
		Timestamp: time.Now().UnixMilli(),
	}
}

func (ct CompressedTrace) JSON() []byte {
	data, _ := json.Marshal(ct)
	return data
}

var _ = strings.Join
