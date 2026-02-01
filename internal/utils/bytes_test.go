package utils

import (
	"testing"
)

func TestLongToBytes(t *testing.T) {
	val := int64(0x1234567890ABCDEF)
	bytes := LongToBytes(val)
	result := BytesToLong(bytes)

	if result != val {
		t.Errorf("LongToBytes/BytesToLong roundtrip failed: got %d, expected %d", result, val)
	}
}

func TestIntegerToBytes(t *testing.T) {
	val := int32(0x12345678)
	bytes := IntegerToBytes(val)
	result := BytesToInteger(bytes)

	if result != val {
		t.Errorf("IntegerToBytes/BytesToInteger roundtrip failed: got %d, expected %d", result, val)
	}
}

func TestShortToBytes(t *testing.T) {
	val := int16(0x1234)
	bytes := ShortToBytes(val)
	result := BytesToShort(bytes)

	if result != val {
		t.Errorf("ShortToBytes/BytesToShort roundtrip failed: got %d, expected %d", result, val)
	}
}

func TestStringToBytes(t *testing.T) {
	str := "hello world"
	bytes := StringToBytes(str)
	result := BytesToString(bytes)

	if result != str {
		t.Errorf("StringToBytes/BytesToString roundtrip failed: got %q, expected %q", result, str)
	}
}

func TestByteArrayToString(t *testing.T) {
	bytes := []byte{1, 2, 3, 4}
	result := ByteArrayToString(bytes)
	expected := "[1, 2, 3, 4]"

	if result != expected {
		t.Errorf("ByteArrayToString failed: got %q, expected %q", result, expected)
	}
}

func TestCopyOfRange(t *testing.T) {
	src := []byte{1, 2, 3, 4, 5}
	result := CopyOfRange(src, 1, 4)
	expected := []byte{2, 3, 4}

	if len(result) != len(expected) {
		t.Errorf("CopyOfRange length mismatch: got %d, expected %d", len(result), len(expected))
	}

	for i := range expected {
		if result[i] != expected[i] {
			t.Errorf("CopyOfRange[%d] = %d, expected %d", i, result[i], expected[i])
		}
	}
}
