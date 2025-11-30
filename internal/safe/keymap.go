package safe

import (
	"sync"
)

type KeyMap[T any] struct {
	m sync.Map
}

func (s *KeyMap[T]) SetKey(key, filed string, value T) {
	v, _ := s.m.LoadOrStore(key, &sync.Map{})
	v.(*sync.Map).Store(filed, value)
}

func (s *KeyMap[T]) GetKey(key, filed string) (t T, ok bool) {
	v, ok := s.m.Load(key)
	if !ok {
		return t, false
	}
	val, ok := v.(*sync.Map).Load(filed)
	if !ok {
		return t, false
	}
	return val.(T), true
}

func (s *KeyMap[T]) RangeKey(key string, f func(string, T) bool) {
	if v, ok := s.m.Load(key); ok {
		v.(*sync.Map).Range(func(k, v any) bool {
			return f(k.(string), v.(T))
		})
	}
}

func (s *KeyMap[T]) DeleteKey(key, filed string) {
	if v, ok := s.m.Load(key); ok {
		v.(*sync.Map).Delete(filed)
	}
}

func (s *KeyMap[T]) Range(f func(string) bool) {
	s.m.Range(func(k, v any) bool {
		return f(k.(string))
	})
}

func (s *KeyMap[T]) Delete(key string) {
	s.m.Delete(key)
}

// Warn: This functoin is expencive. O(n*m) complexity.
func (s *KeyMap[T]) Statistics() map[string]int {
	m := make(map[string]int)
	s.m.Range(func(k, v any) bool {
		key := k.(string)
		m[key] = 0
		v.(*sync.Map).Range(func(k, v any) bool {
			m[key] += 1
			return true
		})
		return true
	})
	return m
}
