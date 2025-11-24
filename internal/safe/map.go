package safe

import (
	"sync"
)

type Map[V any] struct {
	m sync.Map
}

func (m *Map[V]) Set(key string, value V) {
	m.m.Store(key, value)
}
func (m *Map[V]) Get(key string) (actual V, loaded bool) {
	if value, ok := m.m.Load(key); ok {
		return value.(V), true
	}
	return actual, false
}
func (m *Map[V]) Swap(key string, value V) (actual V, loaded bool) {
	if actual, loaded := m.m.Swap(key, value); loaded {
		return actual.(V), true
	}
	return actual, loaded
}
func (m *Map[V]) Range(f func(key string, value V) bool) {
	m.m.Range(func(k, v any) bool {
		return f(k.(string), v.(V))
	})
}
func (m *Map[V]) Delete(key string) {
	m.m.Delete(key)
}
func (m *Map[V]) GetOrSet(key string, value V) (actual V, loaded bool) {
	if actual, loaded := m.m.LoadOrStore(key, value); loaded {
		return actual.(V), true
	}
	return value, false
}
