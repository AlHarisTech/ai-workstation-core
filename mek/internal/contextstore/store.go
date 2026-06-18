package contextstore

import "sync"

// Store holds ephemeral per-execution state — scoped variables, artifacts, session data.
// Lost on MEK termination. No persistence, no snapshot.
type Store struct {
	mu        sync.RWMutex
	variables map[string]interface{}
	artifacts []string
}

func New() *Store {
	return &Store{
		variables: make(map[string]interface{}),
	}
}

func (s *Store) SetVar(key string, value interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.variables[key] = value
}

func (s *Store) GetVar(key string) (interface{}, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.variables[key]
	return v, ok
}

func (s *Store) AddArtifact(ref string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.artifacts = append(s.artifacts, ref)
}

func (s *Store) Artifacts() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.artifacts))
	copy(out, s.artifacts)
	return out
}
