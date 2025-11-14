package broker

import (
	"sync"

	"sutext.github.io/cable/server"
)

type KeyMap struct {
	m sync.Map
}

func (s *KeyMap) Set(key, filed string, value server.Network) {
	v, _ := s.m.LoadOrStore(key, &sync.Map{})
	v.(*sync.Map).Store(filed, value)
}
func (s *KeyMap) Get(key, filed string) (server.Network, bool) {
	v, ok := s.m.Load(key)
	if !ok {
		return 0, false
	}
	val, ok := v.(*sync.Map).Load(filed)
	if !ok {
		return 0, false
	}
	return val.(server.Network), true
}
func (s *KeyMap) Delete(key, filed string) {
	if v, ok := s.m.Load(key); ok {
		v.(*sync.Map).Delete(filed)
	}
}
func (s *KeyMap) Range(key string, f func(string, server.Network) bool) {
	if v, ok := s.m.Load(key); ok {
		v.(*sync.Map).Range(func(k, v any) bool {
			return f(k.(string), v.(server.Network))
		})
	}
}
func (s *KeyMap) Keys() []string {
	var keys []string
	s.m.Range(func(k, v any) bool {
		keys = append(keys, k.(string))
		return true
	})
	return keys
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
