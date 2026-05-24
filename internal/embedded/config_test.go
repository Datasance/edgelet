//go:build (linux && amd64) || (linux && arm64) || (linux && arm) || (linux && riscv64)

package embedded

import (
	"testing"

	"github.com/datasance/edgelet/internal/constants"
)

func TestGenerateManagedCNIConfigUsesManagedConstants(t *testing.T) {
	cfg := generateManagedCNIConfig()
	if got := cfg["name"]; got != constants.EdgeletNetworkName {
		t.Fatalf("managed network name mismatch: got=%v want=%s", got, constants.EdgeletNetworkName)
	}
	plugins, ok := cfg["plugins"].([]map[string]any)
	if !ok || len(plugins) == 0 {
		t.Fatalf("plugins missing from managed config")
	}
	bridge := plugins[0]
	if got := bridge["bridge"]; got != constants.EdgeletBridgeName {
		t.Fatalf("managed bridge mismatch: got=%v want=%s", got, constants.EdgeletBridgeName)
	}
}
