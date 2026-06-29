package runtimeapi

import "testing"

func TestHumanBytesDecimalDockerStyle(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{94900000, "94.9 MB"},
		{340000000, "340 MB"},
		{1590000000, "1.59 GB"},
		{4110000000, "4.11 GB"},
		{1300000000, "1.30 GB"},
	}
	for _, tc := range tests {
		if got := humanBytes(tc.bytes); got != tc.want {
			t.Fatalf("humanBytes(%d) = %q, want %q", tc.bytes, got, tc.want)
		}
	}
}
