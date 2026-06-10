package release

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
)

func IdempotencyKey(commitSHA, versionTag string) string {
	h := sha256.Sum256([]byte(commitSHA + ":" + versionTag))
	return fmt.Sprintf("%x", h)
}

type IdempotencyStore struct {
	mu     sync.RWMutex
	store  map[string]bool
}

func NewIdempotencyStore() *IdempotencyStore {
	return &IdempotencyStore{
		store: make(map[string]bool),
	}
}

func (is *IdempotencyStore) Exists(key string) bool {
	is.mu.RLock()
	defer is.mu.RUnlock()
	return is.store[key]
}

func (is *IdempotencyStore) Mark(key string) {
	is.mu.Lock()
	defer is.mu.Unlock()
	is.store[key] = true
}

func (is *IdempotencyStore) Snapshot() []string {
	is.mu.RLock()
	defer is.mu.RUnlock()
	keys := make([]string, 0, len(is.store))
	for k := range is.store {
		keys = append(keys, k)
	}
	return keys
}

func (is *IdempotencyStore) JSON() string {
	is.mu.RLock()
	defer is.mu.RUnlock()
	raw, _ := json.MarshalIndent(is.store, "", "  ")
	return string(raw)
}
