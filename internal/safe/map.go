package safe

import (
	"sync"
)

// Map is a thread-safe map.
type Map[K comparable, V any] struct {
	m sync.Map
}

func (m *Map[K, V]) Set(key K, value V) {
	m.m.Store(key, value)
}
func (m *Map[K, V]) Get(key K) (actual V, loaded bool) {
	if value, ok := m.m.Load(key); ok {
		return value.(V), true
	}
	return actual, false
}
func (m *Map[K, V]) Swap(key K, value V) (actual V, loaded bool) {
	if actual, loaded := m.m.Swap(key, value); loaded {
		return actual.(V), true
	}
	return actual, loaded
}
func (m *Map[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(func(k, v any) bool {
		return f(k.(K), v.(V))
	})
}
func (m *Map[K, V]) Delete(key K) {
	m.m.Delete(key)
}
func (m *Map[K, V]) GetOrSet(key K, value V) (actual V, loaded bool) {
	if actual, loaded := m.m.LoadOrStore(key, value); loaded {
		return actual.(V), true
	}
	return value, false
}
