package safe

import (
	"sync"
	"sync/atomic"
)

// RMap is Read first thread-safe map implementation.
// It is similar to sync.Map, but provides additional methods to get and set values,
type RMap[K comparable, V any] struct {
	m sync.Map
	l atomic.Int32
}

func (m *RMap[K, V]) Len() int32 {
	return m.l.Load()
}

// Set sets the value for a key.
// If the key is new, the length of the map will be increased by 1. And return true.
// If the key is existing, the length of the map will not be changed. And return false.
func (m *RMap[K, V]) Set(key K, value V) bool {
	if _, ok := m.m.Swap(key, value); ok {
		return false
	}
	m.l.Add(1)
	return true
}
func (m *RMap[K, V]) Get(key K) (actual V, loaded bool) {
	if value, ok := m.m.Load(key); ok {
		return value.(V), true
	}
	return actual, false
}

// Swap swaps the value for a key and returns the previous value if any.
// The loaded result reports whether the key was present.
func (m *RMap[K, V]) Swap(key K, value V) (actual V, loaded bool) {
	if old, ok := m.m.Swap(key, value); ok {
		return old.(V), true
	}
	m.l.Add(1)
	return actual, false
}
func (m *RMap[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}

// Delete deletes the value for a key.
// The deleted result reports whether the key was present.
func (m *RMap[K, V]) Delete(key K) bool {
	if _, loaded := m.m.LoadAndDelete(key); loaded {
		m.l.Add(-1)
		return true
	}
	return false
}

// GetOrSet returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *RMap[K, V]) GetOrSet(key K, value V) (actual V, loaded bool) {
	if val, ok := m.m.LoadOrStore(key, value); ok {
		return val.(V), true
	}
	m.l.Add(1)
	return value, false
}
