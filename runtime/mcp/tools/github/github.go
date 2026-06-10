package github

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/AlHarisTech/ai-workstation-core/runtime/mcp/types"
)

type GitHubMCP struct {
	token  string
	client *http.Client
}

func New() *GitHubMCP {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	return &GitHubMCP{
		token:  token,
		client: &http.Client{},
	}
}

func (gh *GitHubMCP) Name() string { return "github-mcp" }

func (gh *GitHubMCP) Execute(ctx context.Context, req types.MCPRequest) (types.MCPResponse, error) {
	switch req.Action {
	case "create_pr", "pr.create":
		return gh.createPR(req)
	case "list_issues", "issues.list":
		return gh.listIssues(req)
	case "create_issue", "issues.create":
		return gh.createIssue(req)
	default:
		return types.MCPResponse{ID: req.ID, Success: false, Error: "unknown action: " + req.Action}, nil
	}
}

func (gh *GitHubMCP) do(method, url string, body io.Reader) (*http.Response, error) {
	req, _ := http.NewRequest(method, url, body)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ai-workstation-core/1.0")
	if gh.token != "" {
		req.Header.Set("Authorization", "Bearer "+gh.token)
	}
	return gh.client.Do(req)
}

func (gh *GitHubMCP) createPR(req types.MCPRequest) (types.MCPResponse, error) {
	owner, _ := req.Payload["owner"].(string)
	repo, _ := req.Payload["repo"].(string)
	title, _ := req.Payload["title"].(string)
	head, _ := req.Payload["head"].(string)
	base, _ := req.Payload["base"].(string)
	body, _ := req.Payload["body"].(string)

	if owner == "" || repo == "" || title == "" || head == "" {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "owner, repo, title, head are required"}, nil
	}
	if base == "" {
		base = "main"
	}

	data := fmt.Sprintf(`{"title":"%s","head":"%s","base":"%s","body":"%s"}`, title, head, base, body)
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo)
	resp, err := gh.do("POST", url, strings.NewReader(data))
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return types.MCPResponse{ID: req.ID, Success: false, Error: fmt.Sprintf("GitHub API error %d: %s", resp.StatusCode, string(raw))}, nil
	}

	var result map[string]any
	json.Unmarshal(raw, &result)
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"pr": result}}, nil
}

func (gh *GitHubMCP) listIssues(req types.MCPRequest) (types.MCPResponse, error) {
	owner, _ := req.Payload["owner"].(string)
	repo, _ := req.Payload["repo"].(string)
	state, _ := req.Payload["state"].(string)
	if state == "" {
		state = "open"
	}

	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues?state=%s", owner, repo, state)
	resp, err := gh.do("GET", url, nil)
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	var issues []map[string]any
	json.Unmarshal(raw, &issues)

	summaries := make([]map[string]any, 0)
	for _, issue := range issues {
		if _, isPR := issue["pull_request"]; isPR {
			continue
		}
		summaries = append(summaries, map[string]any{
			"number": issue["number"], "title": issue["title"],
			"state": issue["state"], "url": issue["html_url"],
		})
	}
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"issues": summaries, "count": len(summaries)}}, nil
}

func (gh *GitHubMCP) createIssue(req types.MCPRequest) (types.MCPResponse, error) {
	owner, _ := req.Payload["owner"].(string)
	repo, _ := req.Payload["repo"].(string)
	title, _ := req.Payload["title"].(string)
	body, _ := req.Payload["body"].(string)

	if owner == "" || repo == "" || title == "" {
		return types.MCPResponse{ID: req.ID, Success: false, Error: "owner, repo, title are required"}, nil
	}

	data := fmt.Sprintf(`{"title":"%s","body":"%s"}`, title, body)
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)
	resp, err := gh.do("POST", url, strings.NewReader(data))
	if err != nil {
		return types.MCPResponse{ID: req.ID, Success: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return types.MCPResponse{ID: req.ID, Success: false, Error: fmt.Sprintf("GitHub API error %d: %s", resp.StatusCode, string(raw))}, nil
	}

	var result map[string]any
	json.Unmarshal(raw, &result)
	return types.MCPResponse{ID: req.ID, Success: true, Data: map[string]any{"issue": result}}, nil
}
