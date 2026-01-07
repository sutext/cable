package safe

import (
	"sync"
)

// Map is a Write first thread-safe map.
// It is safe for concurrent access by multiple goroutines.
// It is write-first, meaning that writes take precedence over reads.
type Map[K comparable, V any] struct {
	m  map[K]V
	mu sync.RWMutex
}

func NewMap[K comparable, V any](m ...map[K]V) *Map[K, V] {
	if len(m) > 0 {
		return &Map[K, V]{m: m[0]}
	}
	return &Map[K, V]{m: make(map[K]V)}
}

func (m *Map[K, V]) Len() int32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return int32(len(m.m))
}
func (m *Map[K, V]) Set(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m[key] = value
}
func (m *Map[K, V]) Get(key K) (actual V, loaded bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	actual, loaded = m.m[key]
	return actual, loaded
}
func (m *Map[K, V]) Load() map[K]V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.m
}
func (m *Map[K, V]) Store(data map[K]V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.m = data
}

// Swap swaps the value for a key and returns the previous value if any.
// The loaded result reports whether the key was present.
func (m *Map[K, V]) Swap(key K, value V) (actual V, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if actual, loaded = m.m[key]; loaded {
		m.m[key] = value
		return actual, loaded
	}
	m.m[key] = value
	return actual, loaded
}
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.m {
		if !f(k, v) {
			break
		}
	}
}
func (m *Map[K, V]) Delete(key K) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, loaded := m.m[key]; loaded {
		delete(m.m, key)
		return true
	}
	return false
}

// GetOrSet returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *Map[K, V]) GetOrSet(key K, value V) (actual V, loaded bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if actual, loaded := m.m[key]; loaded {
		return actual, loaded
	}
	m.m[key] = value
	return value, false
}
