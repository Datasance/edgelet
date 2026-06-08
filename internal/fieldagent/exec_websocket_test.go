package fieldagent

import (
	"testing"
)

func TestConvertToWebSocketURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "HTTP URL",
			input:    "http://controller.example.com:54421/api/v2/",
			expected: "ws://controller.example.com:54421/api/v2/",
		},
		{
			name:     "HTTPS URL",
			input:    "https://controller.example.com:54421/api/v2/",
			expected: "wss://controller.example.com:54421/api/v2/",
		},
		{
			name:     "Already WebSocket URL",
			input:    "ws://controller.example.com:54421/api/v2/",
			expected: "ws://controller.example.com:54421/api/v2/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToWebSocketURL(tt.input)
			if result != tt.expected {
				t.Errorf("convertToWebSocketURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
