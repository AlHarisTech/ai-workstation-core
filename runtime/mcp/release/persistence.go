package release

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const defaultQueuePath = ".ai/state/release_queue.json"
const defaultIdempotencyPath = ".ai/state/idempotency_store.json"

func PersistQueue(entries []ReleaseEntry, path string) error {
	if path == "" {
		path = defaultQueuePath
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func LoadQueue(path string) ([]ReleaseEntry, error) {
	if path == "" {
		path = defaultQueuePath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []ReleaseEntry{}, nil
		}
		return nil, err
	}
	var entries []ReleaseEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func PersistIdempotencyStore(keys []string, path string) error {
	if path == "" {
		path = defaultIdempotencyPath
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	store := make(map[string]bool)
	for _, k := range keys {
		store[k] = true
	}
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func LoadIdempotencyStore(path string) (map[string]bool, error) {
	if path == "" {
		path = defaultIdempotencyPath
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]bool), nil
		}
		return nil, err
	}
	var store map[string]bool
	if err := json.Unmarshal(raw, &store); err != nil {
		return nil, err
	}
	return store, nil
}

func PersistCompletedEntry(key string, path string) error {
	store, err := LoadIdempotencyStore(path)
	if err != nil {
		return err
	}
	store[key] = true
	raw, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}
