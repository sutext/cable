package safe

import (
	"sync/atomic"
	"testing"
)

func TestMap(t *testing.T) {
	m := Map[string, string]{}
	m.Set("foo", "bar")
	if bar, ok := m.Get("foo"); ok && bar != "bar" {
		t.Error("Expected 'bar', got", bar)
	}
	m.Delete("foo")
	if foo, ok := m.Get("foo"); ok {
		t.Error("Expected nil, got", foo)
	}
}
func BenchmarkMap(b *testing.B) {
	b.Run("safe.Map.Write", func(b *testing.B) {
		m := Map[string, string]{}
		for b.Loop() {
			m.Set("foo", "bar")
		}
	})
	b.Run("safe.XMap.Write", func(b *testing.B) {
		m := NewMap[string, string]()
		for b.Loop() {
			m.Set("foo", "bar")
		}
	})
	b.Run("safe.Map.Read", func(b *testing.B) {
		m := Map[string, string]{}
		m.Set("foo", "bar")
		for b.Loop() {
			m.Get("foo")
		}
	})
	b.Run("safe.XMap.Read", func(b *testing.B) {
		m := NewMap[string, string]()
		m.Set("foo", "bar")
		for b.Loop() {
			m.Get("foo")
		}
	})
}
func BenchmarkKeyMap(b *testing.B) {
	b.Run("safe.KeyMap", func(b *testing.B) {
		m := KeyMap[string]{}
		for b.Loop() {
			m.SetKey("foo", "bar", "baz")
			m.SetKey("foo", "bar1", "baz1")
			m.GetKey("foo", "bar")
			m.DeleteKey("foo", "bar")
		}

	})
}
func BenchmarkAtomic(b *testing.B) {
	b.Run("atomic.Int64", func(b *testing.B) {
		i := atomic.Int64{}
		for b.Loop() {
			i.Add(1)
			i.Add(-1)
		}
	})
	b.Run("int64", func(b *testing.B) {
		var i int64
		for b.Loop() {
			i++
			i--
		}
	})
	b.Run("atomic.Int32", func(b *testing.B) {
		i := atomic.Int32{}
		for b.Loop() {
			i.Add(1)
			i.Add(-1)
		}
	})
	b.Run("int32", func(b *testing.B) {
		var i int32
		for b.Loop() {
			i++
			i--
		}
	})
}
