package config

import (
	"fmt"
	"runtime"
	"strings"
)

// AllowedArchValues are valid config arch settings (FogType source).
var AllowedArchValues = []string{"auto", "amd64", "arm64", "arm", "riscv64"}

// ValidateArch reports whether value is an allowed arch config setting.
func ValidateArch(value string) error {
	v := strings.ToLower(strings.TrimSpace(value))
	for _, allowed := range AllowedArchValues {
		if v == allowed {
			return nil
		}
	}
	return fmt.Errorf("arch must be one of %s", strings.Join(AllowedArchValues, "|"))
}

// ResolveArch maps config arch to a concrete architecture name (auto → runtime GOARCH).
func ResolveArch(value string) string {
	v := strings.ToLower(strings.TrimSpace(value))
	if v != "" && v != "auto" {
		return v
	}
	switch runtime.GOARCH {
	case "amd64", "386":
		return "amd64"
	case "arm64":
		return "arm64"
	case "riscv64":
		return "riscv64"
	case "arm":
		return "arm"
	default:
		return "amd64"
	}
}

// DisplayArch returns the architecture name shown in status/info (never "auto").
func DisplayArch(value string) string {
	return ResolveArch(value)
}

// ArchitectureCode maps arch config to Pot provision FogType (1=amd64, 2=arm64, 3=riscv64, 4=arm).
func ArchitectureCode(value string) int {
	switch ResolveArch(value) {
	case "amd64":
		return 1
	case "arm64":
		return 2
	case "riscv64":
		return 3
	case "arm":
		return 4
	default:
		return 0
	}
}
