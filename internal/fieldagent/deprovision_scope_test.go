package fieldagent

import "testing"

func TestNormalizeDeprovisionScope(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults all", input: "", want: DeprovisionScopeAll},
		{name: "all", input: "all", want: DeprovisionScopeAll},
		{name: "local", input: "local", want: DeprovisionScopeLocal},
		{name: "trimmed local", input: "  LOCAL ", want: DeprovisionScopeLocal},
		{name: "invalid", input: "managed", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeDeprovisionScope(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}
