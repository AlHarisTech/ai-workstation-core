package rir

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anomalyco/mek/pkg/types"
)

func Compile(files []FileNode, projectPath string) *types.RIR {
	project := filepath.Base(projectPath)
	absProject, _ := filepath.Abs(projectPath)
	units := make([]types.Unit, 0)
	allNodes := make([]string, 0)
	edges := make([]types.Edge, 0)
	idSet := make(map[string]bool)

	for _, f := range files {
		if f.IsDir {
			continue
		}

		id := filepath.ToSlash(f.Path)
		if idSet[id] {
			continue
		}
		idSet[id] = true

		unit := types.Unit{
			ID:   id,
			Type: types.UnitTask,
			Binding: types.Binding{
				Contract:  "file",
				Isolation: "inline",
			},
			Dependencies: make([]string, 0),
			DataFlow: types.DataFlow{
				Inputs:  make([]types.DataFlowInput, 0),
				Outputs: []string{"content"},
			},
			Validation: types.Validation{
				Preconditions:  []string{"file_exists"},
				Postconditions: make([]string, 0),
				Invariants:     make([]string, 0),
				FailureModes:   make([]string, 0),
			},
			Scheduling: types.Scheduling{
				Priority: 1,
			},
			Context: types.UnitContext{
				Mode:  "static",
				Tools: make([]string, 0),
			},
			Governance: types.Governance{
				RequiredApprovals: make([]string, 0),
				ChangeScope:       project,
			},
			Activation: types.Activation{
				Condition: "true",
				Requires:  make([]string, 0),
				Optional:  make([]string, 0),
			},
		}

		for _, imp := range f.Imports {
			if !isProjectImport(imp, absProject) {
				continue
			}
			depID := fmt.Sprintf("import:%s", imp)
			if !idSet[depID] {
				idSet[depID] = true
				depUnit := types.Unit{
					ID:   depID,
					Type: types.UnitTask,
					Binding: types.Binding{
						Contract:  "import",
						Isolation: "inline",
					},
					Dependencies: make([]string, 0),
					DataFlow: types.DataFlow{
						Inputs:  make([]types.DataFlowInput, 0),
						Outputs: []string{"resolution"},
					},
					Validation: types.Validation{
						Preconditions:  make([]string, 0),
						Postconditions: make([]string, 0),
						Invariants:     make([]string, 0),
						FailureModes:   make([]string, 0),
					},
					Scheduling: types.Scheduling{Priority: 0},
					Context:    types.UnitContext{Mode: "static", Tools: make([]string, 0)},
					Governance: types.Governance{
						RequiredApprovals: make([]string, 0),
						ChangeScope:       project,
					},
					Activation: types.Activation{
						Condition: "true",
						Requires:  make([]string, 0),
						Optional:  make([]string, 0),
					},
				}
				units = append(units, depUnit)
				allNodes = append(allNodes, depID)
				edges = append(edges, types.Edge{From: id, To: depID, Type: "import"})
			} else {
				edges = append(edges, types.Edge{From: id, To: depID, Type: "import"})
			}
			unit.Dependencies = append(unit.Dependencies, depID)
		}

		units = append(units, unit)
		allNodes = append(allNodes, id)
	}

	sort.Strings(allNodes)

	return &types.RIR{
		Meta: types.RIRMeta{
			SchemaVersion:   "1.0",
			SpecHash:        fmt.Sprintf("mek-rir-%s", project),
			CompilationID:   fmt.Sprintf("gen-%s", project),
			CompiledAt:      "",
			SourceSpec:      project,
			CompilerVersion: "mek-rir-generator-v1",
		},
		ExecutionPlan: types.ExecutionPlan{
			SchedulingModel:   "wave",
			ExecutionStrategy: "deterministic",
			MaxParallelism:    1,
			FailStrategy:      "abort",
			ExecutionMode:     "2",
		},
		Units: units,
		Graph: types.Graph{
			DAG: types.DAG{
				Nodes: allNodes,
				Edges: edges,
			},
			Cycles: make([][]string, 0),
		},
		Assertions: []types.Assertion{
			{
				ID:        "structural-integrity",
				Type:      "graph",
				Predicate: "acyclic",
				OnFailure: "fail",
			},
		},
		FailureModes: make([]types.FailureMode, 0),
		Handoff: types.Handoff{
			OutputArtifacts: []string{"rir.generated.json"},
			Evidence:        make([]string, 0),
		},
	}
}

func isProjectImport(imp, projectPath string) bool {
	return strings.Contains(imp, "easyfit_app") ||
		strings.Contains(imp, "easyfit_shared") ||
		strings.Contains(imp, "easyfit_ui") ||
		strings.Contains(imp, projectPath)
}
