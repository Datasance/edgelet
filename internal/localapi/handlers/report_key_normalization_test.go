package handlers

import "testing"

func TestNormalizeReportKey_CamelCase(t *testing.T) {
	tests := map[string]string{
		"connection-to-controller":   "connectionToController",
		"cpu usage":                  "cpuUsage",
		"gps-coordinates(lat,lon)":   "gpsCoordinates",
		"developer's-mode":           "developerMode",
		"status update frequency":    "statusUpdateFrequency",
		"ready-to-upgrade-scan-frequency": "readyToUpgradeScanFrequency",
	}
	for in, expected := range tests {
		got := normalizeReportKey(in)
		if got != expected {
			t.Fatalf("normalizeReportKey(%q)=%q expected %q", in, got, expected)
		}
	}
}

