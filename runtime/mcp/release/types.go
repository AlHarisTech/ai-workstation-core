package release

type ReleaseInput struct {
	Repo         string   `json:"repo"`
	Version      string   `json:"version"`
	CommitSHA    string   `json:"commit_sha"`
	ReleaseNotes string   `json:"release_notes"`
	Artifacts    []string `json:"artifacts"`
}

type ReleaseOutput struct {
	Success    bool   `json:"success"`
	Tag        string `json:"tag,omitempty"`
	CommitSHA  string `json:"commit_sha,omitempty"`
	ReleaseURL string `json:"release_url,omitempty"`
	Error      string `json:"error,omitempty"`
	TraceID    string `json:"trace_id,omitempty"`
	LatencyMS  int64  `json:"latency_ms"`
}
