package rir

import (
	"path/filepath"
	"sort"
)

type RIR struct {
	Meta  Meta   `json:"meta"`
	Units []Unit `json:"units"`
	Graph Graph  `json:"graph"`
}

type Meta struct {
	SchemaVersion string `json:"schema_version"`
	Project       string `json:"project"`
	GeneratedBy   string `json:"generated_by"`
}

type Unit struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	Deterministic bool   `json:"deterministic"`
}

type Graph struct {
	Nodes []string `json:"nodes"`
	Edges []Edge   `json:"edges"`
}

type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func GenerateRIR(projectPath string, files []FileNode) RIR {
	units := make([]Unit, 0)
	nodes := make([]string, 0)
	edges := make([]Edge, 0)

	for _, f := range files {
		if f.IsDir {
			continue
		}

		id := filepath.ToSlash(f.Path)

		units = append(units, Unit{
			ID:            id,
			Type:          "file",
			Deterministic: true,
		})

		nodes = append(nodes, id)

		for _, imp := range f.Imports {
			if imp != "" {
				edges = append(edges, Edge{
					From: id,
					To:   imp,
				})
			}
		}
	}

	sort.Strings(nodes)

	return RIR{
		Meta: Meta{
			SchemaVersion: "1.0",
			Project:       filepath.Base(projectPath),
			GeneratedBy:   "mek-rir-generator",
		},
		Units: units,
		Graph: Graph{
			Nodes: nodes,
			Edges: edges,
		},
	}
}
