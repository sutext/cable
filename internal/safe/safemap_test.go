package safe

import (
	"fmt"
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
func TestXMap(t *testing.T) {
	m := XMap[string, string]{}
	m.Set("foo", "bar")
	if bar, ok := m.Get("foo"); ok && bar != "bar" {
		t.Error("Expected 'bar', got", bar)
	}
	if m.Len() != 1 {
		t.Error("Expected 1, got", m.Len())
	}
	m.Delete("foo")
	if m.Len() != 0 {
		t.Error("Expected 0, got", m.Len())
	}
	if foo, ok := m.Get("foo"); ok {
		t.Error("Expected nil, got", foo)
	}
}
func BenchmarkMap(b *testing.B) {
	b.Run("safe.XMap", func(b *testing.B) {
		m := XMap[string, string]{}
		for i := 0; b.Loop(); i++ {
			str := fmt.Sprintf("%d", i)
			m.Set(str, str)
			m.Get(str)
			m.Delete(str)
		}
	})
	b.Run("safe.Map", func(b *testing.B) {
		m := Map[string, string]{}
		for i := 0; b.Loop(); i++ {
			str := fmt.Sprintf("%d", i)
			m.Set(str, str)
			m.Get(str)
			m.Delete(str)
		}
	})
	b.Run("map", func(b *testing.B) {
		m := map[string]string{}
		for i := 0; b.Loop(); i++ {
			str := fmt.Sprintf("%d", i)
			m[str] = str
			_ = m[str]
			delete(m, str)
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
	b.Run("sync.XKeyMap", func(b *testing.B) {
		m := XKeyMap[string]{}
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
