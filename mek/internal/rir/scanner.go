package rir

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

type FileNode struct {
	Path    string   `json:"path"`
	IsDir   bool     `json:"is_dir"`
	Imports []string `json:"imports"`
}

var skipDirs = map[string]bool{
	"vendor":       true,
	"build":        true,
	"node_modules": true,
	"coverage":     true,
	".dart_tool":   true,
	".pub-cache":   true,
	".git":         true,
	".idea":        true,
	".vscode":      true,
	"__pycache__":  true,
}

var codeExtensions = map[string]bool{
	".dart":  true,
	".go":    true,
	".yaml":  true,
	".yml":   true,
	".json":  true,
	".md":    true,
	".toml":  true,
	".proto": true,
	".sh":    true,
	".ps1":   true,
}

func ScanProject(root string) ([]FileNode, error) {
	var nodes []FileNode

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve path: %w", err)
	}

	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			if skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			node := FileNode{
				Path:  path,
				IsDir: true,
			}
			nodes = append(nodes, node)
			return nil
		}

		ext := strings.ToLower(filepath.Ext(path))
		if !codeExtensions[ext] {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		node := FileNode{
			Path:    path,
			IsDir:   false,
			Imports: extractImports(string(content)),
		}

		nodes = append(nodes, node)
		return nil
	})

	return nodes, err
}

func extractImports(content string) []string {
	var imports []string

	lines := strings.Split(content, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "import ") {
			imports = append(imports, line)
		}

		if strings.HasPrefix(line, "import '") {
			imports = append(imports, line)
		}
	}

	return imports
}
