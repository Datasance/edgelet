package config

import (
	"os"
	"testing"
)

func TestParseSecureMode(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "off", want: false},
		{value: "false", want: false},
		{value: "0", want: false},
		{value: "no", want: false},
		{value: "on", want: true},
		{value: "true", want: true},
		{value: "1", want: true},
		{value: "yes", want: true},
		{value: "", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			if got := ParseSecureMode(tt.value); got != tt.want {
				t.Fatalf("ParseSecureMode(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestNormalizeSecureModeYAML(t *testing.T) {
	if got := normalizeSecureModeYAML("false"); got != "off" {
		t.Fatalf("normalizeSecureModeYAML(false) = %q, want off", got)
	}
	if got := normalizeSecureModeYAML("on"); got != "on" {
		t.Fatalf("normalizeSecureModeYAML(on) = %q, want on", got)
	}
}

func TestSetConfigSecureModeFalseString(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "setconfig-secure-mode-*.yaml")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	testConfig := `currentProfile: default
profiles:
  default:
    secureMode: "on"
`
	if _, err := tmpFile.WriteString(testConfig); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	_ = tmpFile.Close()

	if err := LoadConfig(tmpFile.Name()); err != nil {
		t.Fatalf("load config: %v", err)
	}

	cfg := GetInstance()
	for _, value := range []any{"false", "0", "no", "off"} {
		cfg.SecureMode = true
		errorsMap := cfg.SetConfig(map[string]any{"sec": value})
		if len(errorsMap) > 0 {
			t.Fatalf("SetConfig errors for %v: %v", value, errorsMap)
		}
		if cfg.SecureMode {
			t.Fatalf("expected SecureMode=false after sec=%v", value)
		}
	}
}
