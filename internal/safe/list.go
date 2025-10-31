package safe

import (
	"slices"
	"sync"
)

type List[Value any] struct {
	mu    sync.RWMutex
	items []Value
}

func NewList[Value any](raw ...[]Value) *List[Value] {
	switch len(raw) {
	case 0:
		return &List[Value]{items: []Value{}}
	case 1:
		return &List[Value]{items: raw[0]}
	default:
		panic("too many arguments")
	}
}
func (l *List[Value]) Raw() []Value {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return slices.Clone(l.items)
}
func (l *List[Value]) SetRaw(items []Value) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = items
}
func (l *List[Value]) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.items)
}
func (l *List[Value]) Get(index int) (Value, bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if index < 0 || index >= len(l.items) {
		var zero Value
		return zero, false
	}
	return l.items[index], true
}
func (l *List[Value]) Set(index int, value Value) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < 0 || index >= len(l.items) {
		return
	}
	l.items[index] = value
}
func (l *List[Value]) Write(fn func([]Value)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	fn(l.items)
}
func (l *List[Value]) Append(value Value) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = append(l.items, value)
}
func (l *List[Value]) Remove(index int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if index < 0 || index >= len(l.items) {
		return
	}
	l.items = append(l.items[:index], l.items[index+1:]...)
}
func (l *List[Value]) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = []Value{}
}
func (l *List[Value]) Range(f func(Value) bool) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, item := range l.items {
		if !f(item) {
			break
		}
	}
}

func (l *List[Value]) Clone() *List[Value] {
	l.mu.RLock()
	defer l.mu.RUnlock()
	list := NewList[Value]()
	list.items = slices.Clone(l.items)
	return list
}
