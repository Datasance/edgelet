package utils

import (
	"bytes"
	"fmt"
)

// CopyOfRange copies a range of bytes from src
func CopyOfRange(src []byte, from, to int) []byte {
	if from < 0 || from >= len(src) || to < from || to > len(src) {
		return []byte{}
	}
	return bytes.Clone(src[from:to])
}

// LongToBytes converts a long (int64) to a byte array
func LongToBytes(x int64) []byte {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(x >> ((8 - i - 1) << 3))
	}
	return b
}

// BytesToLong converts a byte array to a long (int64)
func BytesToLong(b []byte) int64 {
	var result int64
	for _, byteVal := range b {
		result = (result << 8) + int64(byteVal&0xff)
	}
	return result
}

// IntegerToBytes converts an integer (int32) to a byte array
func IntegerToBytes(x int32) []byte {
	b := make([]byte, 4)
	for i := 0; i < 4; i++ {
		b[i] = byte(x >> ((4 - i - 1) << 3))
	}
	return b
}

// BytesToInteger converts a byte array to an integer (int32)
func BytesToInteger(b []byte) int32 {
	var result int32
	for _, byteVal := range b {
		result = (result << 8) + int32(byteVal&0xff)
	}
	return result
}

// ShortToBytes converts a short (int16) to a byte array
func ShortToBytes(x int16) []byte {
	b := make([]byte, 2)
	for i := 0; i < 2; i++ {
		b[i] = byte(x >> ((2 - i - 1) << 3))
	}
	return b
}

// BytesToShort converts a byte array to a short (int16)
func BytesToShort(b []byte) int16 {
	var result int16
	for _, byteVal := range b {
		result = (result << 8) + int16(byteVal&0xff)
	}
	return result
}

// StringToBytes converts a string to a byte array (UTF-8)
func StringToBytes(s string) []byte {
	if s == "" {
		return []byte{}
	}
	return []byte(s)
}

// BytesToString converts a byte array to a string (UTF-8)
func BytesToString(b []byte) string {
	return string(b)
}

// ByteArrayToString returns a string representation of a byte array
// Example: []byte{1, 2, 3, 4} => "[1, 2, 3, 4]"
func ByteArrayToString(b []byte) string {
	if len(b) == 0 {
		return "[]"
	}

	var buf bytes.Buffer
	buf.WriteString("[")
	for i, byteVal := range b {
		if i > 0 {
			buf.WriteString(", ")
		}
		fmt.Fprintf(&buf, "%d", byteVal)
	}
	buf.WriteString("]")
	return buf.String()
}
