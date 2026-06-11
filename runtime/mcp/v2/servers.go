package mcpv2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MCPServer interface {
	Name() string
	Execute(action string, payload map[string]any, ctx MCPContext) (any, error)
}

func parseOp(action string) (string, string) {
	parts := strings.SplitN(action, ".", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], parts[1]
}

func httpClient(timeoutMs int) *http.Client {
	if timeoutMs <= 0 {
		timeoutMs = 30000
	}
	return &http.Client{Timeout: time.Duration(timeoutMs) * time.Millisecond}
}

var _ MCPServer = (*GitServer)(nil)

type GitServer struct{}

func (s *GitServer) Name() string { return "git" }

func (s *GitServer) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)
	workDir := ctx.Workspace.Path
	if workDir == "" {
		return nil, fmt.Errorf("workspace path required for git operations")
	}

	switch op {
	case "status":
		return gitRun(workDir, "status", "--porcelain")
	case "diff":
		return gitRun(workDir, "diff")
	case "log":
		return gitRun(workDir, "log", "--oneline", "-10")
	case "branch":
		return gitRun(workDir, "branch")
	case "commit":
		msg, _ := payload["message"].(string)
		if msg == "" {
			return nil, fmt.Errorf("commit message is required")
		}
		_, err := gitRunRaw(workDir, "add", "-A")
		if err != nil {
			return nil, fmt.Errorf("git add failed: %w", err)
		}
		out, err := gitRunRaw(workDir, "commit", "-m", msg)
		if err != nil {
			return nil, err
		}
		return map[string]any{"output": strings.TrimSpace(string(out))}, nil
	case "push":
		out, err := gitRunRaw(workDir, "push")
		if err != nil {
			return nil, err
		}
		return map[string]any{"output": strings.TrimSpace(string(out))}, nil
	case "tag":
		tag, _ := payload["tag"].(string)
		if tag == "" {
			return nil, fmt.Errorf("tag name is required")
		}
		_, err := gitRunRaw(workDir, "tag", tag)
		if err != nil {
			return nil, err
		}
		return map[string]any{"tag": tag}, nil
	default:
		return nil, fmt.Errorf("unknown git operation: %s", op)
	}
}

func gitRun(workDir string, args ...string) (map[string]any, error) {
	out, err := gitRunRaw(workDir, args...)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"output": strings.TrimSpace(string(out)),
	}, nil
}

func gitRunRaw(workDir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s failed: %w\nstderr: %s", strings.Join(args, " "), err, stderr.String())
	}
	return out, nil
}

var _ MCPServer = (*FilesystemServer)(nil)

type FilesystemServer struct{}

func (s *FilesystemServer) Name() string { return "filesystem" }

func (s *FilesystemServer) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)
	basePath := ctx.Workspace.Path
	if basePath == "" {
		return nil, fmt.Errorf("workspace path required for filesystem operations")
	}
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("invalid workspace path: %w", err)
	}

	switch op {
	case "read":
		path, _ := payload["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		fullPath, err := safeJoin(absBase, path)
		if err != nil {
			return nil, err
		}
		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read failed: %w", err)
		}
		info, _ := os.Stat(fullPath)
		return map[string]any{
			"path":    path,
			"content": string(data),
			"size":    len(data),
			"mode":    info.Mode().String(),
		}, nil

	case "write":
		path, _ := payload["path"].(string)
		content, _ := payload["content"].(string)
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		fullPath, err := safeJoin(absBase, path)
		if err != nil {
			return nil, err
		}
		parent := filepath.Dir(fullPath)
		if err := os.MkdirAll(parent, 0755); err != nil {
			return nil, fmt.Errorf("mkdir failed: %w", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("write failed: %w", err)
		}
		return map[string]any{"path": path, "written": true, "size": len(content)}, nil

	case "delete":
		path, _ := payload["path"].(string)
		if path == "" {
			return nil, fmt.Errorf("path is required")
		}
		fullPath, err := safeJoin(absBase, path)
		if err != nil {
			return nil, err
		}
		if err := os.RemoveAll(fullPath); err != nil {
			return nil, fmt.Errorf("delete failed: %w", err)
		}
		return map[string]any{"path": path, "deleted": true}, nil

	case "list":
		path, _ := payload["path"].(string)
		dirPath := absBase
		if path != "" {
			var err error
			dirPath, err = safeJoin(absBase, path)
			if err != nil {
				return nil, err
			}
		}
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return nil, fmt.Errorf("list failed: %w", err)
		}
		var files []map[string]any
		for _, e := range entries {
			info, _ := e.Info()
			files = append(files, map[string]any{
				"name":  e.Name(),
				"dir":   e.IsDir(),
				"size":  info.Size(),
				"mode":  info.Mode().String(),
			})
		}
		return map[string]any{"path": path, "files": files, "count": len(files)}, nil

	case "search":
		pattern, _ := payload["pattern"].(string)
		if pattern == "" {
			return nil, fmt.Errorf("search pattern is required")
		}
		safePattern := filepath.Join(absBase, pattern)
		matches, err := filepath.Glob(safePattern)
		if err != nil {
			return nil, fmt.Errorf("search failed: %w", err)
		}
		for i, m := range matches {
			rel, err := filepath.Rel(absBase, m)
			if err == nil {
				matches[i] = rel
			}
		}
		return map[string]any{"matches": matches, "count": len(matches)}, nil

	default:
		return nil, fmt.Errorf("unknown filesystem operation: %s", op)
	}
}

func safeJoin(base, target string) (string, error) {
	cleanTarget := filepath.Clean(target)
	if filepath.IsAbs(cleanTarget) {
		if !strings.HasPrefix(cleanTarget, base) {
			return "", fmt.Errorf("path traversal denied: %s is outside workspace", target)
		}
		return cleanTarget, nil
	}
	fullPath := filepath.Join(base, cleanTarget)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("path resolution failed: %w", err)
	}
	if !strings.HasPrefix(absPath, base) {
		return "", fmt.Errorf("path traversal denied: %s resolves outside workspace", target)
	}
	return absPath, nil
}

var _ MCPServer = (*FetchServer)(nil)

type FetchServer struct{}

func (s *FetchServer) Name() string { return "fetch" }

func (s *FetchServer) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)

	switch op {
	case "url", "get":
		url, _ := payload["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("url is required")
		}
		client := httpClient(ctx.TimeoutMs)
		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("http get failed: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response failed: %w", err)
		}
		contentType := resp.Header.Get("Content-Type")
		return map[string]any{
			"url":          url,
			"status":       resp.StatusCode,
			"content_type": contentType,
			"body":         string(body),
			"size":         len(body),
		}, nil

	case "status":
		url, _ := payload["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("url is required")
		}
		client := httpClient(ctx.TimeoutMs)
		resp, err := client.Head(url)
		if err != nil {
			return nil, fmt.Errorf("http head failed: %w", err)
		}
		defer resp.Body.Close()
		return map[string]any{
			"url":    url,
			"status": resp.StatusCode,
			"alive":  resp.StatusCode < 500,
		}, nil

	case "download":
		url, _ := payload["url"].(string)
		if url == "" {
			return nil, fmt.Errorf("url is required")
		}
		client := httpClient(ctx.TimeoutMs)
		resp, err := client.Get(url)
		if err != nil {
			return nil, fmt.Errorf("http get failed: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read response failed: %w", err)
		}
		return map[string]any{
			"url":  url,
			"size": len(body),
			"data": body,
		}, nil

	default:
		return nil, fmt.Errorf("unknown fetch operation: %s", op)
	}
}

var _ MCPServer = (*MemoryServer)(nil)

type MemoryServer struct {
	mu     sync.RWMutex
	store  map[string]string
	file   string
}

func NewMemoryServer(persistencePath string) *MemoryServer {
	s := &MemoryServer{
		store: make(map[string]string),
		file:  persistencePath,
	}
	if s.file != "" {
		s.load()
	}
	return s
}

func (s *MemoryServer) Name() string { return "memory" }

func (s *MemoryServer) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)

	s.mu.Lock()
	defer s.mu.Unlock()

	switch op {
	case "store":
		key, _ := payload["key"].(string)
		value, _ := payload["value"].(string)
		if key == "" {
			return nil, fmt.Errorf("key is required")
		}
		s.store[key] = value
		s.persist()
		return map[string]any{"key": key, "stored": true}, nil

	case "retrieve":
		key, _ := payload["key"].(string)
		if key == "" {
			return nil, fmt.Errorf("key is required")
		}
		value, ok := s.store[key]
		if !ok {
			return nil, fmt.Errorf("key not found: %s", key)
		}
		return map[string]any{"key": key, "value": value}, nil

	case "delete":
		key, _ := payload["key"].(string)
		if key == "" {
			return nil, fmt.Errorf("key is required")
		}
		delete(s.store, key)
		s.persist()
		return map[string]any{"key": key, "deleted": true}, nil

	case "list":
		keys := make([]string, 0, len(s.store))
		for k := range s.store {
			keys = append(keys, k)
		}
		return map[string]any{"keys": keys, "count": len(keys)}, nil

	default:
		return nil, fmt.Errorf("unknown memory operation: %s", op)
	}
}

func (s *MemoryServer) persist() {
	if s.file == "" {
		return
	}
	data, err := json.Marshal(s.store)
	if err != nil {
		return
	}
	os.MkdirAll(filepath.Dir(s.file), 0755)
	os.WriteFile(s.file, data, 0644)
}

func (s *MemoryServer) load() {
	data, err := os.ReadFile(s.file)
	if err != nil {
		return
	}
	json.Unmarshal(data, &s.store)
}

var _ MCPServer = (*GitHubServer)(nil)

type GitHubServer struct {
	client *http.Client
}

func NewGitHubServer() *GitHubServer {
	return &GitHubServer{client: httpClient(30000)}
}

func (s *GitHubServer) Name() string { return "github" }

func (s *GitHubServer) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)
	owner, _ := payload["owner"].(string)
	repo, _ := payload["repo"].(string)

	token := ""
	if ctx.TenantID != "" {
		token = ctx.TenantID
	}

	switch op {
	case "read", "repo":
		path, _ := payload["path"].(string)
		if owner == "" || repo == "" || path == "" {
			return nil, fmt.Errorf("owner, repo, and path are required")
		}
		if token == "" {
			return nil, fmt.Errorf("GitHub token required (set via auth)")
		}
		return s.ghGet(token, fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", owner, repo, path))

	case "list_issues":
		if owner == "" || repo == "" {
			return nil, fmt.Errorf("owner and repo are required")
		}
		if token == "" {
			return nil, fmt.Errorf("GitHub token required")
		}
		state, _ := payload["state"].(string)
		url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)
		if state != "" {
			url += "?state=" + state
		}
		return s.ghGet(token, url)

	case "create_issue":
		if owner == "" || repo == "" {
			return nil, fmt.Errorf("owner and repo are required")
		}
		if token == "" {
			return nil, fmt.Errorf("GitHub token required")
		}
		title, _ := payload["title"].(string)
		body, _ := payload["body"].(string)
		if title == "" {
			return nil, fmt.Errorf("title is required")
		}
		return s.ghPost(token, fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo),
			map[string]any{"title": title, "body": body})

	case "create_pr":
		if owner == "" || repo == "" {
			return nil, fmt.Errorf("owner and repo are required")
		}
		if token == "" {
			return nil, fmt.Errorf("GitHub token required")
		}
		title, _ := payload["title"].(string)
		head, _ := payload["head"].(string)
		base, _ := payload["base"].(string)
		if title == "" || head == "" || base == "" {
			return nil, fmt.Errorf("title, head, and base are required")
		}
		return s.ghPost(token, fmt.Sprintf("https://api.github.com/repos/%s/%s/pulls", owner, repo),
			map[string]any{"title": title, "head": head, "base": base})

	case "create_release":
		if owner == "" || repo == "" {
			return nil, fmt.Errorf("owner and repo are required")
		}
		if token == "" {
			return nil, fmt.Errorf("GitHub token required")
		}
		tag, _ := payload["tag"].(string)
		name, _ := payload["name"].(string)
		if tag == "" {
			return nil, fmt.Errorf("tag is required")
		}
		body := map[string]any{"tag_name": tag, "name": name}
		return s.ghPost(token, fmt.Sprintf("https://api.github.com/repos/%s/%s/releases", owner, repo), body)

	default:
		return nil, fmt.Errorf("unknown github operation: %s", op)
	}
}

func (s *GitHubServer) ghGet(token, url string) (any, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result any
	json.Unmarshal(body, &result)
	return map[string]any{
		"status": resp.StatusCode,
		"data":   result,
	}, nil
}

func (s *GitHubServer) ghPost(token, url string, bodyPayload map[string]any) (any, error) {
	bodyBytes, _ := json.Marshal(bodyPayload)
	req, _ := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github api error: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var result any
	json.Unmarshal(body, &result)
	return map[string]any{
		"status": resp.StatusCode,
		"data":   result,
	}, nil
}

var _ MCPServer = (*Context7Server)(nil)

type Context7Server struct {
	mu        sync.RWMutex
	store     map[string]any
	client    *http.Client
	apiKey    string
	supabaseURL string
	supabaseKey string
}

func NewContext7Server() *Context7Server {
	s := &Context7Server{
		store:     map[string]any{},
		client:    httpClient(10000),
		apiKey:    os.Getenv("CONTEXT7_API_KEY"),
		supabaseURL: os.Getenv("SUPABASE_URL"),
		supabaseKey: os.Getenv("SUPABASE_ANON_KEY"),
	}

	if s.apiKey == "" && s.supabaseURL != "" && s.supabaseKey != "" {
		if key, err := s.fetchKeyFromSupabase(); err == nil && key != "" {
			s.apiKey = key
			log.Printf("context7: api key fetched from supabase cloud")
		} else if err != nil {
			log.Printf("context7: supabase fetch failed (fallback to env): %v", err)
		}
	}

	return s
}

func (s *Context7Server) Name() string { return "context7" }

func (s *Context7Server) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)

	switch op {
	case "query":
		key, _ := payload["key"].(string)
		if key == "" {
			key = "default"
		}
		serviceURL, _ := payload["service_url"].(string)
		if serviceURL != "" {
			return s.remoteQuery(serviceURL, key, ctx)
		}
		if s.apiKey != "" {
			return s.remoteQuery("https://api.context7.com/v1", key, ctx)
		}
		return s.localQuery(key)

	case "store":
		key, _ := payload["key"].(string)
		value := payload["value"]
		if key == "" {
			return nil, fmt.Errorf("key is required")
		}
		s.mu.Lock()
		s.store[key] = value
		s.mu.Unlock()
		return map[string]any{"key": key, "stored": true}, nil

	case "resolve":
		key, _ := payload["key"].(string)
		if key == "" {
			key = "session"
		}
		s.mu.RLock()
		val, exists := s.store[key]
		s.mu.RUnlock()
		if !exists {
			val = map[string]any{
				"session_id": ctx.SessionID,
				"trace_id":   ctx.TraceID,
				"workspace":  ctx.Workspace.Path,
				"resolved":   true,
			}
			s.mu.Lock()
			s.store[key] = val
			s.mu.Unlock()
		}
		return map[string]any{"key": key, "context": val}, nil

	default:
		return nil, fmt.Errorf("unknown context7 operation: %s", op)
	}
}

func (s *Context7Server) fetchKeyFromSupabase() (string, error) {
	url := fmt.Sprintf("%s/rest/v1/mcp_secrets?service=eq.context7&select=api_key", s.supabaseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("apikey", s.supabaseKey)
	req.Header.Set("Authorization", "Bearer "+s.supabaseKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("supabase request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("supabase returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	var rows []struct {
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}
	if len(rows) == 0 {
		return "", fmt.Errorf("no key found in supabase")
	}
	return rows[0].APIKey, nil
}

func (s *Context7Server) localQuery(key string) (any, error) {
	s.mu.RLock()
	val, exists := s.store[key]
	s.mu.RUnlock()

	if !exists {
		val = map[string]any{
			"key":         key,
			"description": "deterministic context response for " + key,
			"type":        "local",
			"resolved":    true,
		}
		s.mu.Lock()
		s.store[key] = val
		s.mu.Unlock()
	}
	return map[string]any{"key": key, "data": val}, nil
}

func (s *Context7Server) remoteQuery(serviceURL, key string, ctx MCPContext) (any, error) {
	url := fmt.Sprintf("%s/context?key=%s&session=%s", strings.TrimRight(serviceURL, "/"), key, ctx.SessionID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	if s.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.apiKey)
		req.Header.Set("X-Api-Key", s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("context7 service unreachable: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read context7 response failed: %w", err)
	}
	var result any
	json.Unmarshal(body, &result)
	source := "remote"
	if s.apiKey != "" {
		source = "cloud"
	}
	return map[string]any{"key": key, "data": result, "source": source}, nil
}

var _ MCPServer = (*PostgresAdapter)(nil)

type PostgresAdapter struct {
	connString string
}

func NewPostgresAdapter() *PostgresAdapter {
	return &PostgresAdapter{}
}

func (p *PostgresAdapter) Name() string { return "postgres" }

func (p *PostgresAdapter) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)

	switch op {
	case "query":
		sql, _ := payload["sql"].(string)
		if sql == "" {
			return nil, fmt.Errorf("sql is required")
		}
		return p.execSQL(sql, true)

	case "execute":
		sql, _ := payload["sql"].(string)
		if sql == "" {
			return nil, fmt.Errorf("sql is required")
		}
		return p.execSQL(sql, false)

	case "list_tables":
		return p.execSQL("SELECT table_name FROM information_schema.tables WHERE table_schema = 'public' ORDER BY table_name", true)

	default:
		return nil, fmt.Errorf("unknown postgres operation: %s", op)
	}
}

func (p *PostgresAdapter) execSQL(sql string, returnRows bool) (any, error) {
	if p.connString == "" {
		log.Printf("[postgres] no connection string configured, logging sql: %s", sql)
		return map[string]any{
			"status": "logged",
			"sql":    sql,
			"notice": "no database connection configured — query was logged, not executed",
		}, nil
	}
	return p.execRemote(sql, returnRows)
}

func (p *PostgresAdapter) execRemote(sql string, returnRows bool) (any, error) {
	body := map[string]any{"query": sql}
	if !returnRows {
		body["execute"] = true
	}
	payload, _ := json.Marshal(body)
	url := p.connString + "/query"
	resp, err := http.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("postgres query failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result any
	json.Unmarshal(respBody, &result)
	return map[string]any{
		"status": resp.StatusCode,
		"result": result,
	}, nil
}

var _ MCPServer = (*ChromaAdapter)(nil)

type ChromaAdapter struct {
	baseURL   string
	apiKey    string
	tenant    string
	database  string
	cloudHost string
	cloudPort int
	client    *http.Client
}

func NewChromaAdapter() *ChromaAdapter {
	a := &ChromaAdapter{
		client:    &http.Client{Timeout: 10 * time.Second},
		apiKey:    os.Getenv("CHROMA_API_KEY"),
		tenant:    os.Getenv("CHROMA_TENANT"),
		database:  os.Getenv("CHROMA_DATABASE"),
		cloudHost: "api.trychroma.com",
		cloudPort: 443,
	}
	if h := os.Getenv("CHROMA_HOST"); h != "" {
		a.cloudHost = h
	}
	if p := os.Getenv("CHROMA_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err != nil || port < 1 || port > 65535 {
			a.cloudPort = 443
		} else {
			a.cloudPort = port
		}
	}
	a.baseURL = fmt.Sprintf("https://%s:%d", a.cloudHost, a.cloudPort)
	baseURL := os.Getenv("CHROMA_URL")
	if baseURL != "" {
		a.baseURL = baseURL
	}
	if a.apiKey != "" {
		log.Printf("chroma: cloud connection configured for tenant=%s database=%s host=%s", a.tenant, a.database, a.cloudHost)
	}
	return a
}

func (c *ChromaAdapter) v2path(path string) string {
	return fmt.Sprintf("%s/api/v2/tenants/%s/databases/%s/%s", c.baseURL, c.tenant, c.database, path)
}

func (c *ChromaAdapter) authHeader() http.Header {
	h := http.Header{}
	if c.apiKey != "" {
		h.Set("X-Chroma-Token", c.apiKey)
	}
	return h
}

func (c *ChromaAdapter) doReq(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	for k, v := range c.authHeader() {
		req.Header[k] = v
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.client.Do(req)
}

func (c *ChromaAdapter) Name() string { return "chroma" }

func (c *ChromaAdapter) Execute(action string, payload map[string]any, ctx MCPContext) (any, error) {
	_, op := parseOp(action)

	switch op {
	case "store":
		docID, _ := payload["id"].(string)
		docText, _ := payload["document"].(string)
		collection, _ := payload["collection"].(string)
		if docID == "" || docText == "" {
			return nil, fmt.Errorf("id and document are required")
		}
		return c.storeDoc(collection, docID, docText, ctx)

	case "query":
		query, _ := payload["query"].(string)
		collection, _ := payload["collection"].(string)
		if query == "" {
			return nil, fmt.Errorf("query is required")
		}
		return c.QueryKnowledge(collection, query)

	case "delete":
		docID, _ := payload["id"].(string)
		collection, _ := payload["collection"].(string)
		if docID == "" {
			return nil, fmt.Errorf("id is required")
		}
		return c.deleteDoc(collection, docID)

	case "list_collections":
		return c.listCollections()

	default:
		return nil, fmt.Errorf("unknown chroma operation: %s", op)
	}
}

func (c *ChromaAdapter) storeDoc(collection, docID, docText string, ctx MCPContext) (any, error) {
	if c.apiKey == "" || c.tenant == "" || c.database == "" {
		log.Printf("[chroma] no cloud credentials configured, logging document %s", docID)
		return map[string]any{
			"id":     docID,
			"status": "logged",
			"notice": "no chroma cloud connection configured — document was logged, not stored",
		}, nil
	}
	col := collection
	if col == "" {
		col = "mcp_execution_memory"
	}
	body := map[string]any{
		"documents": []map[string]any{
			{
				"id":       docID,
				"document": docText,
				"metadata": map[string]string{
					"session_id": ctx.SessionID,
					"trace_id":   ctx.TraceID,
					"collection": col,
				},
			},
		},
	}
	payload, _ := json.Marshal(body)
	url := c.v2path(fmt.Sprintf("collections/%s/add", col))
	resp, err := c.doReq("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("chroma store failed: %w", err)
	}
	defer resp.Body.Close()
	return map[string]any{"id": docID, "status": resp.StatusCode, "collection": col}, nil
}

func (c *ChromaAdapter) queryDocs(collection, query string) (any, error) {
	if c.apiKey == "" || c.tenant == "" || c.database == "" {
		return map[string]any{}, nil
	}
	col := collection
	if col == "" {
		col = "mcp_execution_memory"
	}
	body := map[string]any{
		"query":    []string{query},
		"n_results": 10,
	}
	payload, _ := json.Marshal(body)
	url := c.v2path(fmt.Sprintf("collections/%s/query", col))
	resp, err := c.doReq("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("chroma query failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result any
	json.Unmarshal(respBody, &result)
	return map[string]any{"query": query, "results": result}, nil
}

func (c *ChromaAdapter) QueryKnowledge(collection, query string) (any, error) {
	return c.queryDocs(collection, query)
}

func (c *ChromaAdapter) deleteDoc(collection, docID string) (any, error) {
	if c.apiKey == "" || c.tenant == "" || c.database == "" {
		return map[string]any{"id": docID, "status": "logged"}, nil
	}
	col := collection
	if col == "" {
		col = "mcp_execution_memory"
	}
	body := map[string]any{"ids": []string{docID}}
	payload, _ := json.Marshal(body)
	url := c.v2path(fmt.Sprintf("collections/%s/delete", col))
	resp, err := c.doReq("POST", url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("chroma delete failed: %w", err)
	}
	defer resp.Body.Close()
	return map[string]any{"id": docID, "status": resp.StatusCode}, nil
}

func (c *ChromaAdapter) listCollections() (any, error) {
	if c.apiKey == "" || c.tenant == "" || c.database == "" {
		return map[string]any{
			"collections": []string{"mcp_execution_memory"},
			"status":      "simulated",
		}, nil
	}
	url := c.v2path("collections")
	resp, err := c.doReq("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("chroma list collections failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	var result any
	json.Unmarshal(respBody, &result)
	return map[string]any{"collections": result}, nil
}
