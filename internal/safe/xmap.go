package safe

import (
	"sync"
	"sync/atomic"
)

// Map is a thread-safe map.
type XMap[K comparable, V any] struct {
	m sync.Map
	l atomic.Int32
}

func (m *XMap[K, V]) Len() int32 {
	return m.l.Load()
}
func (m *XMap[K, V]) Set(key K, value V) {
	if _, loaded := m.m.Swap(key, value); !loaded {
		m.l.Add(1)
	}
}
func (m *XMap[K, V]) Get(key K) (actual V, loaded bool) {
	if value, ok := m.m.Load(key); ok {
		return value.(V), true
	}
	return actual, false
}

// Swap swaps the value for a key and returns the previous value if any.
// The loaded result reports whether the key was present.
func (m *XMap[K, V]) Swap(key K, value V) (actual V, loaded bool) {
	if actual, loaded := m.m.Swap(key, value); loaded {
		return actual.(V), true
	}
	m.l.Add(1)
	return actual, loaded
}
func (m *XMap[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}
func (m *XMap[K, V]) Delete(key K) {
	if _, loaded := m.m.LoadAndDelete(key); loaded {
		m.l.Add(-1)
	}
}

// GetOrSet returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *XMap[K, V]) GetOrSet(key K, value V) (actual V, loaded bool) {
	if actual, loaded := m.m.LoadOrStore(key, value); loaded {
		return actual.(V), true
	}
	m.l.Add(1)
	return value, false
}
