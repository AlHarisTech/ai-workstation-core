package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type GitMCP struct {
	workDir string
}

func New(workDir string) *GitMCP {
	return &GitMCP{workDir: workDir}
}

func (g *GitMCP) Name() string { return "git-mcp" }

func (g *GitMCP) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	switch req.Action {
	case "status":
		return g.status(req)
	case "commit":
		return g.commit(req)
	case "diff":
		return g.diff(req)
	case "branch":
		return g.branch(req)
	case "log":
		return g.log(req)
	default:
		return types.MCPResponse{ID: req.ID, Success: false, Error: "unknown action: " + req.Action}, nil
	}
}

func (g *GitMCP) run(cmd string, args ...string) (string, error) {
	c := exec.Command(cmd, args...)
	c.Dir = g.workDir
	out, err := c.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("%s: %s", err.Error(), string(out))
	}
	return string(out), nil
}

func (g *GitMCP) status(req types.MCPRequest) (types.MCPResponse, error) {
	out, err := g.run("git", "status", "--porcelain")
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"changes": lines, "count": len(lines)}}, nil
}

func (g *GitMCP) commit(req types.MCPRequest) (types.MCPResponse, error) {
	msg, _ := req.Payload["message"].(string)
	files, _ := req.Payload["files"].([]any)
	if msg == "" {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "message is required"}, nil
	}
	if len(files) > 0 {
		args := append([]string{"add"}, toStringSlice(files)...)
		if _, err := g.run("git", args...); err != nil {
			return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
		}
	}
	out, err := g.run("git", "commit", "-m", msg)
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"output": strings.TrimSpace(out)}}, nil
}

func (g *GitMCP) diff(req types.MCPRequest) (types.MCPResponse, error) {
	out, err := g.run("git", "diff")
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	if len(out) > 50000 {
		out = out[:50000]
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"diff": out}}, nil
}

func (g *GitMCP) branch(req types.MCPRequest) (types.MCPResponse, error) {
	out, err := g.run("git", "branch", "--list")
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	branches := make([]string, 0, len(lines))
	for _, l := range lines {
		branches = append(branches, strings.TrimPrefix(l, "* "))
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"branches": branches, "count": len(branches)}}, nil
}

func (g *GitMCP) log(req types.MCPRequest) (types.MCPResponse, error) {
	n, _ := req.Payload["count"].(float64)
	if n <= 0 {
		n = 10
	}
	args := []string{"log", "--oneline", fmt.Sprintf("-n%d", int(n))}
	out, err := g.run("git", args...)
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"commits": lines, "count": len(lines)}}, nil
}

func toStringSlice(a []any) []string {
	s := make([]string, len(a))
	for i, v := range a {
		s[i] = fmt.Sprint(v)
	}
	return s
}
