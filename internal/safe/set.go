package safe

import "sync"

type Set[Value comparable] struct {
	mu sync.RWMutex
	mp map[Value]struct{}
}

func NewSet[Value comparable](v ...Value) *Set[Value] {
	mp := make(map[Value]struct{})
	for _, value := range v {
		mp[value] = struct{}{}
	}
	return &Set[Value]{mp: mp}
}
func (s *Set[Value]) Has(value Value) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.mp[value]
	return ok
}
func (s *Set[Value]) Add(value Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mp[value] = struct{}{}
}

func (s *Set[Value]) Del(value Value) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.mp, value)
}
func (s *Set[Value]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.mp)
}
func (s *Set[Value]) Range(f func(Value) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for key := range s.mp {
		if !f(key) {
			break
		}
	}
}
