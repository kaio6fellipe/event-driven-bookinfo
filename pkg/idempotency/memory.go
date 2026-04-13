package idempotency

import (
	"context"
	"sync"
)

// MemoryStore is an in-memory Store. Suitable for local dev and tests.
// Lost on process restart.
type MemoryStore struct {
	mu   sync.Mutex
	keys map[string]struct{}
}

// NewMemoryStore creates a new in-memory Store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{keys: make(map[string]struct{})}
}

// CheckAndRecord implements Store.
func (m *MemoryStore) CheckAndRecord(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.keys[key]; ok {
		return true, nil
	}
	m.keys[key] = struct{}{}
	return false, nil
}
