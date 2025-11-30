package safe

type XKeyMap[T any] struct {
	m XMap[string, *XMap[string, T]]
}

func (s *XKeyMap[T]) LenKey(key string) int {
	if v, ok := s.m.Get(key); ok {
		return int(v.Len())
	}
	return 0
}
func (s *XKeyMap[T]) SetKey(key, filed string, value T) {
	v, _ := s.m.GetOrSet(key, &XMap[string, T]{})
	v.Set(filed, value)
}

func (s *XKeyMap[T]) GetKey(key, filed string) (t T, ok bool) {
	v, ok := s.m.Get(key)
	if !ok {
		return t, false
	}
	val, ok := v.Get(filed)
	if !ok {
		return t, false
	}
	return val, true
}

func (s *XKeyMap[T]) RangeKey(key string, f func(string, T) bool) {
	if v, ok := s.m.Get(key); ok {
		v.Range(func(k string, v T) bool {
			return f(k, v)
		})
	}
}

func (s *XKeyMap[T]) DeleteKey(key, filed string) {
	if v, ok := s.m.Get(key); ok {
		v.Delete(filed)
		if v.Len() == 0 {
			s.m.Delete(key)
		}
	}
}
func (s *XKeyMap[T]) Len() int {
	return int(s.m.Len())
}
func (s *XKeyMap[T]) Set(key string, value *XMap[string, T]) {
	s.m.Set(key, value)
}

func (s *XKeyMap[T]) Get(key string) (value *XMap[string, T], ok bool) {
	return s.m.Get(key)
}
func (s *XKeyMap[T]) Range(f func(string, *XMap[string, T]) bool) {
	s.m.Range(func(k string, v *XMap[string, T]) bool {
		return f(k, v)
	})
}

func (s *XKeyMap[T]) Delete(key string) {
	s.m.Delete(key)
}
