package safe

import (
	"maps"
	"sync"
)

type Map[Key comparable, Value any] struct {
	mu sync.RWMutex
	mp map[Key]Value
}

func NewMap[Key comparable, Value any](raw ...map[Key]Value) *Map[Key, Value] {
	switch len(raw) {
	case 0:
		return &Map[Key, Value]{
			mp: make(map[Key]Value),
		}
	case 1:
		return &Map[Key, Value]{
			mp: raw[0],
		}
	default:
		panic("too many arguments")
	}
}
func (m *Map[Key, Value]) Raw() map[Key]Value {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mp
}
func (m *Map[Key, Value]) SetRaw(mp map[Key]Value) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mp = mp
}

func (m *Map[Key, Value]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.mp)
}
func (m *Map[Key, Value]) Get(key Key) (Value, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	value, ok := m.mp[key]
	return value, ok
}
func (m *Map[Key, Value]) Set(key Key, value Value) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mp[key] = value
}
func (m *Map[Key, Value]) Write(fn func(mp map[Key]Value)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	fn(m.mp)
}
func (m *Map[Key, Value]) Read(fn func(mp map[Key]Value)) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	fn(m.mp)
}
func (m *Map[Key, Value]) Delete(key Key) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.mp, key)
}
func (m *Map[Key, Value]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mp = make(map[Key]Value)
}
func (m *Map[Key, Value]) Exists(key Key) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.mp[key]
	return ok
}
func (m *Map[Key, Value]) Keys() []Key {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]Key, 0, len(m.mp))
	for key := range m.mp {
		keys = append(keys, key)
	}
	return keys
}
func (m *Map[Key, Value]) Values() []Value {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values := make([]Value, 0, len(m.mp))
	for _, value := range m.mp {
		values = append(values, value)
	}
	return values
}
func (m *Map[Key, Value]) IsEmpty() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.mp) == 0
}
func (m *Map[Key, Value]) Copy() *Map[Key, Value] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	newMap := NewMap[Key, Value]()
	maps.Copy(newMap.mp, m.mp)
	return newMap
}
func (m *Map[Key, Value]) Merge(other *Map[Key, Value]) {
	m.mu.Lock()
	defer m.mu.Unlock()
	maps.Copy(m.mp, other.mp)
}
func (m *Map[Key, Value]) Union(other *Map[Key, Value]) *Map[Key, Value] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	unionMap := NewMap(m.mp)
	maps.Copy(unionMap.mp, other.mp)
	return unionMap
}
func (m *Map[Key, Value]) Range(f func(key Key, value Value) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for key, value := range m.mp {
		if !f(key, value) {
			break
		}
	}
}
func (m *Map[Key, Value]) Filter(f func(key Key, value Value) bool) *Map[Key, Value] {
	m.mu.RLock()
	defer m.mu.RUnlock()
	newMap := NewMap[Key, Value]()
	for key, value := range m.mp {
		if f(key, value) {
			newMap.Set(key, value)
		}
	}
	return newMap
}
