package filesystem

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type FilesystemMCP struct{}

func New() *FilesystemMCP {
	return &FilesystemMCP{}
}

func (f *FilesystemMCP) Name() string { return "filesystem-mcp" }

func (f *FilesystemMCP) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	switch req.Action {
	case "read":
		return f.read(req)
	case "write":
		return f.write(req)
	case "list":
		return f.list(req)
	case "search":
		return f.search(req)
	default:
		return types.MCPResponse{ID: req.ID, Success: false, Error: fmt.Sprintf("unknown action: %s", req.Action)}, nil
	}
}

func (f *FilesystemMCP) read(req types.MCPRequest) (types.MCPResponse, error) {
	path, _ := req.Payload["path"].(string)
	if path == "" {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "path is required"}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	truncated := string(data)
	if len(truncated) > 50000 {
		truncated = truncated[:50000]
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"content": truncated, "path": path, "size": len(data)}}, nil
}

func (f *FilesystemMCP) write(req types.MCPRequest) (types.MCPResponse, error) {
	path, _ := req.Payload["path"].(string)
	content, _ := req.Payload["content"].(string)
	if path == "" || content == "" {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "path and content are required"}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"written": true, "path": path, "size": len(content)}}, nil
}

func (f *FilesystemMCP) list(req types.MCPRequest) (types.MCPResponse, error) {
	dir, _ := req.Payload["path"].(string)
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	files := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		files = append(files, map[string]any{"name": e.Name(), "is_dir": e.IsDir()})
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"path": dir, "entries": files, "count": len(files)}}, nil
}

func (f *FilesystemMCP) search(req types.MCPRequest) (types.MCPResponse, error) {
	pattern, _ := req.Payload["pattern"].(string)
	dir, _ := req.Payload["path"].(string)
	if pattern == "" {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "pattern is required"}, nil
	}
	if dir == "" {
		dir = "."
	}
	var matches []string
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(p)); matched {
			matches = append(matches, p)
		}
		return nil
	})
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"pattern": pattern, "matches": matches, "count": len(matches)}}, nil
}

var _ = strings.Join
