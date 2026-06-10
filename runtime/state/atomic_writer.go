package state

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type AtomicWriter struct {
	mu sync.Mutex
}

func NewAtomicWriter() *AtomicWriter {
	return &AtomicWriter{}
}

func (aw *AtomicWriter) WriteJSON(filePath string, data interface{}) error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	dir := os.TempDir()
	name := filepath.Base(filePath)

	tmpFile, err := os.CreateTemp(dir, name+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	encoder := json.NewEncoder(tmpFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(data); err != nil {
		tmpFile.Close()
		return err
	}
	tmpFile.Close()

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err
	}
	return os.Rename(tmpPath, filePath)
}

func (aw *AtomicWriter) ReadJSON(filePath string, v interface{}) error {
	aw.mu.Lock()
	defer aw.mu.Unlock()

	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
