package statusreporter

import (
	"strings"
	"testing"
)

func TestGetStatusReport_IncludesAvailableNetworkInterfacesAfterSystemTotalCPU(t *testing.T) {
	report := GetInstance().GetStatusReport()

	totalCPUIdx := strings.Index(report, "System Total CPU")
	availableInterfacesIdx := strings.Index(report, "Available Network Interfaces")
	if totalCPUIdx == -1 {
		t.Fatalf("expected System Total CPU line in report, got:\n%s", report)
	}
	if availableInterfacesIdx == -1 {
		t.Fatalf("expected Available Network Interfaces line in report, got:\n%s", report)
	}
	if availableInterfacesIdx < totalCPUIdx {
		t.Fatalf("expected Available Network Interfaces after System Total CPU, got:\n%s", report)
	}
}
