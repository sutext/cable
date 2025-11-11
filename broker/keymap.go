package broker

import (
	"sync"
)

type KeyMap struct {
	m sync.Map
}

func (s *KeyMap) Set(key, filed string, value uint8) {
	v, _ := s.m.LoadOrStore(key, &sync.Map{})
	v.(*sync.Map).Store(filed, value)
}
func (s *KeyMap) Get(key, filed string) (uint8, bool) {
	v, ok := s.m.Load(key)
	if !ok {
		return 0, false
	}
	val, ok := v.(*sync.Map).Load(filed)
	if !ok {
		return 0, false
	}
	return val.(uint8), true
}
func (s *KeyMap) Delete(key, filed string) {
	if v, ok := s.m.Load(key); ok {
		v.(*sync.Map).Delete(filed)
	}
}
func (s *KeyMap) Range(key string, f func(string, uint8) bool) {
	if v, ok := s.m.Load(key); ok {
		v.(*sync.Map).Range(func(k, v any) bool {
			return f(k.(string), v.(uint8))
		})
	}
}
func (s *KeyMap) Dump() map[string]int {
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
