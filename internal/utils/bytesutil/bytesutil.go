package bytesutil

import (
	"encoding/binary"
	"unicode/utf8"
)

// CopyOfRange copies a range of bytes from src, similar to Java Arrays.copyOfRange
func CopyOfRange(src []byte, from, to int) []byte {
	if from < 0 || from >= len(src) || to < from || to > len(src) {
		return []byte{}
	}
	return src[from:to]
}

// LongToBytes converts a long (int64) to a byte array (big-endian)
func LongToBytes(x int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(x)) // #nosec G115 -- wire-format conversion; value range is controlled by caller
	return b
}

// BytesToLong converts a byte array to a long (int64) (big-endian)
func BytesToLong(bytes []byte) int64 {
	if len(bytes) < 8 {
		// Pad with zeros if needed
		padded := make([]byte, 8)
		copy(padded[8-len(bytes):], bytes)
		bytes = padded
	}
	return int64(binary.BigEndian.Uint64(bytes)) // #nosec G115 -- wire-format conversion; value range is controlled by caller
}

// IntegerToBytes converts an integer (int32) to a byte array (big-endian)
func IntegerToBytes(x int) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(x)) // #nosec G115 -- wire-format conversion; value range is controlled by caller
	return b
}

// BytesToInteger converts a byte array to an integer (int32) (big-endian)
func BytesToInteger(bytes []byte) int {
	if len(bytes) < 4 {
		// Pad with zeros if needed
		padded := make([]byte, 4)
		copy(padded[4-len(bytes):], bytes)
		bytes = padded
	}
	return int(binary.BigEndian.Uint32(bytes)) // #nosec G115 -- wire-format conversion; value range is controlled by caller
}

// ShortToBytes converts a short (int16) to a byte array (big-endian)
func ShortToBytes(x int16) []byte {
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, uint16(x)) // #nosec G115 -- wire-format conversion; value range is controlled by caller
	return b
}

// BytesToShort converts a byte array to a short (int16) (big-endian)
func BytesToShort(bytes []byte) int16 {
	if len(bytes) < 2 {
		// Pad with zeros if needed
		padded := make([]byte, 2)
		copy(padded[2-len(bytes):], bytes)
		bytes = padded
	}
	return int16(binary.BigEndian.Uint16(bytes)) // #nosec G115 -- wire-format conversion; value range is controlled by caller
}

// StringToBytes converts a string to a byte array (UTF-8)
func StringToBytes(s string) []byte {
	if s == "" {
		return []byte{}
	}
	return []byte(s)
}

// BytesToString converts a byte array to a string (UTF-8)
func BytesToString(bytes []byte) string {
	if len(bytes) == 0 {
		return ""
	}
	// Validate UTF-8
	if !utf8.Valid(bytes) {
		// If invalid UTF-8, return as-is (may contain binary data)
		return string(bytes)
	}
	return string(bytes)
}

// ByteArrayToString returns a string representation of a byte array
// Example: [1, 2, 3, 4] => "[1, 2, 3, 4]"
func ByteArrayToString(bytes []byte) string {
	if len(bytes) == 0 {
		return "[]"
	}

	result := "["
	for i, b := range bytes {
		if i > 0 {
			result += ", "
		}
		// Convert byte to string representation
		if b < 10 {
			result += string(rune('0' + b))
		} else if b < 100 {
			result += string(rune('0'+(b/10))) + string(rune('0'+(b%10)))
		} else {
			// For bytes > 99, use three digits
			result += string(rune('0'+(b/100))) + string(rune('0'+((b/10)%10))) + string(rune('0'+(b%10)))
		}
	}
	result += "]"
	return result
}
