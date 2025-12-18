package safe

type KeyMap[T any] struct {
	m Map[string, *Map[string, T]]
}

func (m *KeyMap[T]) LenKey(key string) int {
	if v, ok := m.m.Get(key); ok {
		return int(v.Len())
	}
	return 0
}
func (m *KeyMap[T]) SetKey(key, filed string, value T) {
	v, _ := m.m.GetOrSet(key, &Map[string, T]{})
	v.Set(filed, value)
}

func (m *KeyMap[T]) GetKey(key, filed string) (t T, ok bool) {
	v, ok := m.m.Get(key)
	if !ok {
		return t, false
	}
	val, ok := v.Get(filed)
	if !ok {
		return t, false
	}
	return val, true
}

func (m *KeyMap[T]) RangeKey(key string, f func(string, T) bool) {
	if v, ok := m.m.Get(key); ok {
		v.Range(func(k string, v T) bool {
			return f(k, v)
		})
	}
}

func (m *KeyMap[T]) DeleteKey(key, filed string) {
	if v, ok := m.m.Get(key); ok {
		v.Delete(filed)
		if v.Len() == 0 {
			m.m.Delete(key)
		}
	}
}
func (m *KeyMap[T]) Len() int32 {
	return m.m.Len()
}
func (m *KeyMap[T]) Set(key string, value *Map[string, T]) {
	m.m.Set(key, value)
}

func (m *KeyMap[T]) Get(key string) (value *Map[string, T], ok bool) {
	return m.m.Get(key)
}
func (m *KeyMap[T]) Range(f func(string, *Map[string, T]) bool) {
	m.m.Range(func(k string, v *Map[string, T]) bool {
		return f(k, v)
	})
}

func (m *KeyMap[T]) Delete(key string) {
	m.m.Delete(key)
}
