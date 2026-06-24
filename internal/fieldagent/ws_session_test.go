package fieldagent

import "testing"

func TestExecSessionRunning(t *testing.T) {
	tests := []struct {
		name   string
		status any
		want   bool
	}{
		{name: "nil", status: nil, want: false},
		{name: "false", status: false, want: false},
		{name: "true", status: true, want: true},
		{name: "wrong type", status: "running", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := execSessionRunning(tt.status); got != tt.want {
				t.Fatalf("execSessionRunning(%v) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
