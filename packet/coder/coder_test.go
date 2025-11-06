package coder

import (
	"testing"
)

func TestCoderInt(t *testing.T) {
	testInt(int8(-100), t)
	testInt(int16(-10000), t)
	testInt(int32(-1000000000), t)
	testInt(int64(-1000000000000000000), t)
	testInt(uint8(255), t)
	testInt(uint16(65535), t)
	testInt(uint32(4294967295), t)
	testInt(uint64(18446744073709551615), t)
}
func TestCoderVarint(t *testing.T) {
	testVarint(100, t)
	testVarint(10000, t)
	testVarint(1000000000, t)
	testVarint(1000000000000000000, t)
}
func TestCoderString(t *testing.T) {
	testString("hello world", t)
	testString("你好，世界", t)
	testString("こんにちは世界", t)
	testString("안녕하세요 세계", t)
}

func testInt(data any, t *testing.T) {
	var buufer = &coder{}
	switch d := data.(type) {
	case int8:
		buufer.WriteInt8(d)
		new, _ := buufer.ReadInt8()
		if new != d {
			t.Error("new != d")
		}
	case int16:
		buufer.WriteInt16(d)
		new, _ := buufer.ReadInt16()
		if new != d {
			t.Error("new != d")
		}
	case int32:
		buufer.WriteInt32(d)
		new, _ := buufer.ReadInt32()
		if new != d {
			t.Error("new != d")
		}
	case int64:
		buufer.WriteInt64(d)
		new, _ := buufer.ReadInt64()
		if new != d {
			t.Error("new != d")
		}
	case uint8:
		buufer.WriteUInt8(d)
		new, _ := buufer.ReadUInt8()
		if new != d {
			t.Error("new != d")
		}
	case uint16:
		buufer.WriteUInt16(d)
		new, _ := buufer.ReadUInt16()
		if new != d {
			t.Error("new != d")
		}
	case uint32:
		buufer.WriteUInt32(d)
		new, _ := buufer.ReadUInt32()
		if new != d {
			t.Error("new != d")
		}
	case uint64:
		buufer.WriteUInt64(d)
		new, _ := buufer.ReadUInt64()
		if new != d {
			t.Error("new != d")
		}
	default:
		t.Error("unknown type")
	}
}
func testVarint(i uint64, t *testing.T) {
	var b = &coder{}
	b.WriteVarint(i)
	new, _ := b.ReadVarint()
	if new != i {
		t.Error("new != i")
	}
}
func testString(s string, t *testing.T) {
	var b = &coder{}
	b.WriteString(s)
	new, _ := b.ReadString()
	if new != s {
		t.Error("new != s")
	}
}
