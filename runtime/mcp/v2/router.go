package mcpv2

import "fmt"

type Capability struct {
	Server       string   `json:"server"`
	Capabilities []string `json:"capabilities"`
	Version      string   `json:"version"`
}

type Router struct {
	registry map[ActionType]*Capability
}

func NewRouter() *Router {
	r := &Router{registry: make(map[ActionType]*Capability)}
	r.registerDefaults()
	return r
}

func (r *Router) registerDefaults() {
	r.registry[ActionGit] = &Capability{Server: "git", Capabilities: []string{"status", "diff", "commit", "push", "branch", "log", "tag"}, Version: "1.0"}
	r.registry[ActionFilesystem] = &Capability{Server: "filesystem", Capabilities: []string{"read", "write", "list", "search", "delete"}, Version: "1.0"}
	r.registry[ActionMemory] = &Capability{Server: "memory", Capabilities: []string{"store", "retrieve", "delete", "list"}, Version: "1.0"}
	r.registry[ActionGitHub] = &Capability{Server: "github", Capabilities: []string{"read", "repo", "list_issues", "create_issue", "create_pr", "create_release", "push", "tag"}, Version: "1.0"}
	r.registry[ActionFetch] = &Capability{Server: "fetch", Capabilities: []string{"url", "get", "status", "download"}, Version: "1.0"}
	r.registry[ActionContext7] = &Capability{Server: "context7", Capabilities: []string{"query", "store", "resolve"}, Version: "1.0"}
	r.registry[ActionPostgres] = &Capability{Server: "postgres", Capabilities: []string{"query", "execute", "list_tables"}, Version: "1.0"}
	r.registry[ActionChromaDB] = &Capability{Server: "chroma", Capabilities: []string{"store", "query", "delete", "list_collections"}, Version: "1.0"}
}

func (r *Router) ListAll() []*Capability {
	caps := make([]*Capability, 0, len(r.registry))
	for _, cap := range r.registry {
		caps = append(caps, cap)
	}
	return caps
}

func (r *Router) Resolve(actionType ActionType, operation string) (*Capability, error) {
	cap, ok := r.registry[actionType]
	if !ok {
		return nil, fmt.Errorf("no server registered for action: %s", actionType)
	}
	for _, c := range cap.Capabilities {
		if c == operation {
			return cap, nil
		}
	}
	return nil, fmt.Errorf("operation %s not supported by server %s", operation, cap.Server)
}
