package rir

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/anomalyco/mek/pkg/types"
)

type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("RIR validation failed: [%s] %s", e.Code, e.Message)
}

func Load(path string) (*types.RIR, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read RIR file: %w", err)
	}

	var rir types.RIR
	if err := json.Unmarshal(data, &rir); err != nil {
		return nil, fmt.Errorf("failed to parse RIR JSON: %w", err)
	}

	if err := Validate(&rir); err != nil {
		return nil, err
	}

	return &rir, nil
}

func Validate(rir *types.RIR) error {
	// I-001: schema_version check
	if rir.Meta.SchemaVersion != "1.0" {
		return &ValidationError{
			Code:    "SCHEMA_VERSION_MISMATCH",
			Message: fmt.Sprintf("expected schema_version 1.0, got %s", rir.Meta.SchemaVersion),
		}
	}

	// MEK v1 supports Mode 2 only
	if rir.ExecutionPlan.ExecutionMode != "2" {
		return &ValidationError{
			Code:    "UNSUPPORTED_MODE",
			Message: fmt.Sprintf("MEK v1 supports Mode 2 only, got mode %s", rir.ExecutionPlan.ExecutionMode),
		}
	}

	// I-004: max_parallelism > 0
	if rir.ExecutionPlan.MaxParallelism <= 0 {
		return &ValidationError{
			Code:    "INVALID_MAX_PARALLELISM",
			Message: "max_parallelism must be > 0",
		}
	}

	// I-005: no unit.id duplication
	ids := make(map[string]bool)
	for _, u := range rir.Units {
		if ids[u.ID] {
			return &ValidationError{
				Code:    "DUPLICATE_UNIT_ID",
				Message: fmt.Sprintf("duplicate unit id: %s", u.ID),
			}
		}
		ids[u.ID] = true
	}

	// I-006: spec_hash non-empty
	if rir.Meta.SpecHash == "" {
		return &ValidationError{
			Code:    "MISSING_SPEC_HASH",
			Message: "spec_hash must be non-empty",
		}
	}

	// MEK: checkpoint nodes forbidden
	for _, u := range rir.Units {
		if u.Type == types.UnitCheckpoint {
			return &ValidationError{
				Code:    "CHECKPOINT_FORBIDDEN_IN_MEK",
				Message: fmt.Sprintf("checkpoint node %s is forbidden in MEK v1 (Mode 2 only)", u.ID),
			}
		}
	}

	// I-002: every dependency must resolve
	unitIDs := make(map[string]bool)
	for _, u := range rir.Units {
		unitIDs[u.ID] = true
	}
	for _, u := range rir.Units {
		for _, dep := range u.Dependencies {
			if !unitIDs[dep] {
				return &ValidationError{
					Code:    "UNRESOLVED_DEPENDENCY",
					Message: fmt.Sprintf("unit %s depends on unknown unit %s", u.ID, dep),
				}
			}
		}
	}

	// V-005: Gate nodes must have branches + default
	for _, u := range rir.Units {
		if u.Type == types.UnitGate {
			if u.Gate == nil || len(u.Gate.Branches) == 0 {
				return &ValidationError{
					Code:    "GATE_MISSING_BRANCHES",
					Message: fmt.Sprintf("gate node %s must have branches", u.ID),
				}
			}
			if u.Gate.Default == "" {
				return &ValidationError{
					Code:    "GATE_MISSING_DEFAULT",
					Message: fmt.Sprintf("gate node %s must have a default branch", u.ID),
				}
			}
			// V-014: at most one ESCALATE branch per gate
			escalateCount := 0
			for _, b := range u.Gate.Branches {
				if b.Target == "ESCALATE" {
					escalateCount++
				}
			}
			if escalateCount > 1 {
				return &ValidationError{
					Code:    "MULTIPLE_ESCALATE_BRANCHES",
					Message: fmt.Sprintf("gate node %s has %d ESCALATE branches (max 1)", u.ID, escalateCount),
				}
			}
		}
	}

	return nil
}
