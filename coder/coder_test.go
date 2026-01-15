package coder

import (
	"testing"
)

// TestBasicTypes tests encoding and decoding of basic types
func TestBasicTypes(t *testing.T) {
	// Test UInt8
	t.Run("UInt8", func(t *testing.T) {
		encoder := NewEncoder()
		original := uint8(42)
		encoder.WriteUInt8(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadUInt8()
		if err != nil {
			t.Fatalf("ReadUInt8 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("UInt8 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test UInt16
	t.Run("UInt16", func(t *testing.T) {
		encoder := NewEncoder()
		original := uint16(12345)
		encoder.WriteUInt16(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadUInt16()
		if err != nil {
			t.Fatalf("ReadUInt16 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("UInt16 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test UInt32
	t.Run("UInt32", func(t *testing.T) {
		encoder := NewEncoder()
		original := uint32(123456789)
		encoder.WriteUInt32(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadUInt32()
		if err != nil {
			t.Fatalf("ReadUInt32 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("UInt32 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test UInt64
	t.Run("UInt64", func(t *testing.T) {
		encoder := NewEncoder()
		original := uint64(123456789012345)
		encoder.WriteUInt64(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadUInt64()
		if err != nil {
			t.Fatalf("ReadUInt64 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("UInt64 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test Int8
	t.Run("Int8", func(t *testing.T) {
		encoder := NewEncoder()
		original := int8(-42)
		encoder.WriteInt8(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadInt8()
		if err != nil {
			t.Fatalf("ReadInt8 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("Int8 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test Int16
	t.Run("Int16", func(t *testing.T) {
		encoder := NewEncoder()
		original := int16(-12345)
		encoder.WriteInt16(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadInt16()
		if err != nil {
			t.Fatalf("ReadInt16 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("Int16 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test Int32
	t.Run("Int32", func(t *testing.T) {
		encoder := NewEncoder()
		original := int32(-123456789)
		encoder.WriteInt32(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadInt32()
		if err != nil {
			t.Fatalf("ReadInt32 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("Int32 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test Int64
	t.Run("Int64", func(t *testing.T) {
		encoder := NewEncoder()
		original := int64(-123456789012345)
		encoder.WriteInt64(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadInt64()
		if err != nil {
			t.Fatalf("ReadInt64 failed: %v", err)
		}
		if decoded != original {
			t.Errorf("Int64 mismatch: expected %v, got %v", original, decoded)
		}
	})

	// Test Bool
	t.Run("Bool", func(t *testing.T) {
		// Test true
		encoder := NewEncoder()
		encoder.WriteBool(true)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadBool()
		if err != nil {
			t.Fatalf("ReadBool failed: %v", err)
		}
		if decoded != true {
			t.Errorf("Bool mismatch: expected true, got %v", decoded)
		}

		// Test false
		encoder = NewEncoder()
		encoder.WriteBool(false)
		
		decoder = NewDecoder(encoder.Bytes())
		decoded, err = decoder.ReadBool()
		if err != nil {
			t.Fatalf("ReadBool failed: %v", err)
		}
		if decoded != false {
			t.Errorf("Bool mismatch: expected false, got %v", decoded)
		}
	})

	// Test Varint
	t.Run("Varint", func(t *testing.T) {
		testCases := []uint64{0, 1, 100, 1000, 1000000, 18446744073709551615}
		for _, original := range testCases {
			encoder := NewEncoder()
			encoder.WriteVarint(original)
			
			decoder := NewDecoder(encoder.Bytes())
			decoded, err := decoder.ReadVarint()
			if err != nil {
				t.Fatalf("ReadVarint failed for %v: %v", original, err)
			}
			if decoded != original {
				t.Errorf("Varint mismatch: expected %v, got %v", original, decoded)
			}
		}
	})
}

// TestComplexTypes tests encoding and decoding of complex types
func TestComplexTypes(t *testing.T) {
	// Test String
	t.Run("String", func(t *testing.T) {
		testCases := []string{"", "hello", "hello world", "中文测试"}
		for _, original := range testCases {
			encoder := NewEncoder()
			encoder.WriteString(original)
			
			decoder := NewDecoder(encoder.Bytes())
			decoded, err := decoder.ReadString()
			if err != nil {
				t.Fatalf("ReadString failed for %q: %v", original, err)
			}
			if decoded != original {
				t.Errorf("String mismatch: expected %q, got %q", original, decoded)
			}
		}
	})

	// Test Data ([]byte)
	t.Run("Data", func(t *testing.T) {
		testCases := [][]byte{nil, {}, {1, 2, 3}, {100, 200, 255}}
		for _, original := range testCases {
			encoder := NewEncoder()
			encoder.WriteData(original)
			
			decoder := NewDecoder(encoder.Bytes())
			decoded, err := decoder.ReadData()
			if err != nil {
				t.Fatalf("ReadData failed for %v: %v", original, err)
			}
			// Compare byte slices
			if len(decoded) != len(original) {
				t.Errorf("Data length mismatch: expected %d, got %d", len(original), len(decoded))
				continue
			}
			for i := range original {
				if decoded[i] != original[i] {
					t.Errorf("Data mismatch at index %d: expected %v, got %v", i, original[i], decoded[i])
					break
				}
			}
		}
	})

	// Test StrMap (map[string]string)
	t.Run("StrMap", func(t *testing.T) {
		testCases := []map[string]string{
			nil,
			{},
			{"key1": "value1"},
			{"key1": "value1", "key2": "value2", "key3": "value3"},
		}
		for _, original := range testCases {
			encoder := NewEncoder()
			encoder.WriteStrMap(original)
			
			decoder := NewDecoder(encoder.Bytes())
			decoded, err := decoder.ReadStrMap()
			if err != nil {
				t.Fatalf("ReadStrMap failed for %v: %v", original, err)
			}
			// Compare maps
			if len(decoded) != len(original) {
				t.Errorf("StrMap length mismatch: expected %d, got %d", len(original), len(decoded))
				continue
			}
			for k, v := range original {
				if decoded[k] != v {
					t.Errorf("StrMap mismatch for key %q: expected %q, got %q", k, v, decoded[k])
				}
			}
		}
	})

	// Test Strings ([]string)
	t.Run("Strings", func(t *testing.T) {
		testCases := [][]string{
			nil,
			{},
			{"hello"},
			{"hello", "world", "test"},
		}
		for _, original := range testCases {
			encoder := NewEncoder()
			encoder.WriteStrings(original)
			
			decoder := NewDecoder(encoder.Bytes())
			decoded, err := decoder.ReadStrings()
			if err != nil {
				t.Fatalf("ReadStrings failed for %v: %v", original, err)
			}
			// Compare string slices
			if len(decoded) != len(original) {
				t.Errorf("Strings length mismatch: expected %d, got %d", len(original), len(decoded))
				continue
			}
			for i := range original {
				if decoded[i] != original[i] {
					t.Errorf("Strings mismatch at index %d: expected %q, got %q", i, original[i], decoded[i])
					break
				}
			}
		}
	})

	// Test UInt8Map (map[uint8]string)
	t.Run("UInt8Map", func(t *testing.T) {
		testCases := []map[uint8]string{
			nil,
			{},
			{1: "value1"},
			{1: "value1", 2: "value2", 3: "value3"},
		}
		for _, original := range testCases {
			encoder := NewEncoder()
			encoder.WriteUInt8Map(original)
			
			decoder := NewDecoder(encoder.Bytes())
			decoded, err := decoder.ReadUInt8Map()
			if err != nil {
				t.Fatalf("ReadUInt8Map failed for %v: %v", original, err)
			}
			// Compare maps
			if len(decoded) != len(original) {
				t.Errorf("UInt8Map length mismatch: expected %d, got %d", len(original), len(decoded))
				continue
			}
			for k, v := range original {
				if decoded[k] != v {
					t.Errorf("UInt8Map mismatch for key %d: expected %q, got %q", k, v, decoded[k])
				}
			}
		}
	})
}

// TestBytes tests WriteBytes and ReadBytes
func TestBytes(t *testing.T) {
	testCases := [][]byte{nil, {}, {1, 2, 3}, {100, 200, 255}}
	for _, original := range testCases {
		encoder := NewEncoder()
		encoder.WriteBytes(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadBytes(uint64(len(original)))
		if err != nil {
			t.Fatalf("ReadBytes failed for %v: %v", original, err)
		}
		// Compare byte slices
		if len(decoded) != len(original) {
			t.Errorf("Bytes length mismatch: expected %d, got %d", len(original), len(decoded))
			continue
		}
		for i := range original {
			if decoded[i] != original[i] {
				t.Errorf("Bytes mismatch at index %d: expected %v, got %v", i, original[i], decoded[i])
				break
			}
		}
	}
}

// TestReadAll tests ReadAll method
func TestReadAll(t *testing.T) {
	testCases := [][]byte{nil, {}, {1, 2, 3}, {100, 200, 255}}
	for _, original := range testCases {
		encoder := NewEncoder()
		encoder.WriteBytes(original)
		
		decoder := NewDecoder(encoder.Bytes())
		decoded, err := decoder.ReadAll()
		if err != nil {
			t.Fatalf("ReadAll failed for %v: %v", original, err)
		}
		// Compare byte slices
		if len(decoded) != len(original) {
			t.Errorf("ReadAll length mismatch: expected %d, got %d", len(original), len(decoded))
			continue
		}
		for i := range original {
			if decoded[i] != original[i] {
				t.Errorf("ReadAll mismatch at index %d: expected %v, got %v", i, original[i], decoded[i])
				break
			}
		}
	}
}

// Define a test struct that implements Codable
type TestStruct struct {
	ID   uint32
	Name string
	Tags []string
	Data map[string]string
}

// Implement Encodable
func (ts *TestStruct) WriteTo(encoder Encoder) error {
	encoder.WriteUInt32(ts.ID)
	encoder.WriteString(ts.Name)
	encoder.WriteStrings(ts.Tags)
	encoder.WriteStrMap(ts.Data)
	return nil
}

// Implement Decodable
func (ts *TestStruct) ReadFrom(decoder Decoder) error {
	var err error
	if ts.ID, err = decoder.ReadUInt32(); err != nil {
		return err
	}
	if ts.Name, err = decoder.ReadString(); err != nil {
		return err
	}
	if ts.Tags, err = decoder.ReadStrings(); err != nil {
		return err
	}
	if ts.Data, err = decoder.ReadStrMap(); err != nil {
		return err
	}
	return nil
}

// TestCodable tests the Codable interface with Marshal and Unmarshal
func TestCodable(t *testing.T) {
	// Test Marshal and Unmarshal
	original := &TestStruct{
		ID:   123,
		Name: "test",
		Tags: []string{"tag1", "tag2"},
		Data: map[string]string{"key1": "value1", "key2": "value2"},
	}

	// Marshal
	bytes, err := Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Unmarshal
	decoded := &TestStruct{}
	err = Unmarshal(bytes, decoded)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// Compare results
	if decoded.ID != original.ID {
		t.Errorf("ID mismatch: expected %d, got %d", original.ID, decoded.ID)
	}
	if decoded.Name != original.Name {
		t.Errorf("Name mismatch: expected %q, got %q", original.Name, decoded.Name)
	}
	// Compare Tags
	if len(decoded.Tags) != len(original.Tags) {
		t.Errorf("Tags length mismatch: expected %d, got %d", len(original.Tags), len(decoded.Tags))
	} else {
		for i := range original.Tags {
			if decoded.Tags[i] != original.Tags[i] {
				t.Errorf("Tags mismatch at index %d: expected %q, got %q", i, original.Tags[i], decoded.Tags[i])
			}
		}
	}
	// Compare Data
	if len(decoded.Data) != len(original.Data) {
		t.Errorf("Data length mismatch: expected %d, got %d", len(original.Data), len(decoded.Data))
	} else {
		for k, v := range original.Data {
			if decoded.Data[k] != v {
				t.Errorf("Data mismatch for key %q: expected %q, got %q", k, v, decoded.Data[k])
			}
		}
	}
}

// TestCombinedTypes tests encoding and decoding multiple types in sequence
func TestCombinedTypes(t *testing.T) {
	// Create encoder and write multiple types
	encoder := NewEncoder()
	encoder.WriteUInt8(42)
	encoder.WriteString("hello")
	encoder.WriteBool(true)
	encoder.WriteInt32(-12345)
	encoder.WriteStrings([]string{"a", "b", "c"})

	// Create decoder and read back in the same order
	decoder := NewDecoder(encoder.Bytes())

	// Read and verify each type
	u8, err := decoder.ReadUInt8()
	if err != nil || u8 != 42 {
		t.Errorf("Expected UInt8 42, got %v (err: %v)", u8, err)
	}

	str, err := decoder.ReadString()
	if err != nil || str != "hello" {
		t.Errorf("Expected String 'hello', got %q (err: %v)", str, err)
	}

	b, err := decoder.ReadBool()
	if err != nil || b != true {
		t.Errorf("Expected Bool true, got %v (err: %v)", b, err)
	}

	i32, err := decoder.ReadInt32()
	if err != nil || i32 != -12345 {
		t.Errorf("Expected Int32 -12345, got %v (err: %v)", i32, err)
	}

	strs, err := decoder.ReadStrings()
	expectedStrs := []string{"a", "b", "c"}
	if err != nil || len(strs) != len(expectedStrs) {
		t.Errorf("Expected Strings %v, got %v (err: %v)", expectedStrs, strs, err)
	} else {
		for i := range expectedStrs {
			if strs[i] != expectedStrs[i] {
				t.Errorf("Strings mismatch at index %d: expected %q, got %q", i, expectedStrs[i], strs[i])
			}
		}
	}
}
