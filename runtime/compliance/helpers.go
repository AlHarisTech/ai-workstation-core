package compliance

import (
	"os"
	"path/filepath"
)

func tempWorkspaceRoot() string {
	dir, err := os.MkdirTemp("", "compliance-test-*")
	if err != nil {
		dir = "/tmp/compliance_test"
		os.MkdirAll(dir, 0755)
	}
	os.MkdirAll(filepath.Join(dir, ".ai", "governance", "audit"), 0755)
	os.MkdirAll(filepath.Join(dir, ".ai", "state", "traces"), 0755)
	os.MkdirAll(filepath.Join(dir, ".ai", "state", "sessions"), 0755)
	os.MkdirAll(filepath.Join(dir, ".ai", "state", "snapshots"), 0755)
	return dir
}
