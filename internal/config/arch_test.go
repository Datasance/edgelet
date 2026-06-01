package config

import "testing"

func TestValidateArch(t *testing.T) {
	for _, valid := range AllowedArchValues {
		if err := ValidateArch(valid); err != nil {
			t.Fatalf("ValidateArch(%q): %v", valid, err)
		}
	}
	if err := ValidateArch("x86"); err == nil {
		t.Fatal("expected x86 to be rejected")
	}
}

func TestArchitectureCode(t *testing.T) {
	tests := map[string]int{
		"amd64":   1,
		"arm64":   2,
		"riscv64": 3,
		"arm":     4,
	}
	for arch, want := range tests {
		if got := ArchitectureCode(arch); got != want {
			t.Fatalf("ArchitectureCode(%q)=%d want %d", arch, got, want)
		}
	}
}

func TestResolveArchAuto(t *testing.T) {
	got := ResolveArch("auto")
	switch got {
	case "amd64", "arm64", "riscv64", "arm":
	default:
		t.Fatalf("unexpected resolved arch %q", got)
	}
}

func TestDisplayArchNeverAuto(t *testing.T) {
	if got := DisplayArch("auto"); got == "auto" || got == "" {
		t.Fatalf("DisplayArch(auto)=%q", got)
	}
}
