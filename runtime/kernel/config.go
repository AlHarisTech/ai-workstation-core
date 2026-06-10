package kernel

type KernelConfig struct {
	WorkspaceRoot string `json:"workspace_root"`
	WorkerCount   int    `json:"workers"`
	QueueSize     int    `json:"queue_size"`
	LogPath       string `json:"log_path"`
	StatePath     string `json:"state_path"`
	RegistryPath  string `json:"registry_path"`
	PolicyPath    string `json:"policy_path"`
}

func DefaultConfig() KernelConfig {
	return KernelConfig{
		WorkerCount:  4,
		QueueSize:    128,
		LogPath:      ".ai/governance/audit/gateway.log",
		StatePath:    ".ai/state",
		RegistryPath: "runtime/tools/definitions.yaml",
		PolicyPath:   "runtime/governance/policies/runtime.yaml",
	}
}
