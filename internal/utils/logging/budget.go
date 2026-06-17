package logging

import "os"

const (
	controlPlaneLogShare = 0.60
	dataPlaneLogShare    = 0.40
)

// SeriesRole identifies which daemon log series receives a budget share.
type SeriesRole int

const (
	// SeriesControlPlane is edgelet.service (basename edgelet).
	SeriesControlPlane SeriesRole = iota
	// SeriesDataPlane is edgelet-containerd.service (basename edgelet-containerd).
	SeriesDataPlane
)

// RuntimeSplitFromEnv reports whether the control plane runs attach-only (EDGELET_RUNTIME_SPLIT=1).
func RuntimeSplitFromEnv() bool {
	return os.Getenv("EDGELET_RUNTIME_SPLIT") == "1"
}

// DaemonLogBudgetMB returns the MiB budget for one daemon log series.
// totalGiB is config logLimit; runtimeSplit applies the 60/40 split for embedded split mode.
func DaemonLogBudgetMB(totalGiB float64, role SeriesRole, runtimeSplit bool) int {
	totalMB := int(totalGiB * 1024)
	if totalMB < 1 {
		totalMB = 1
	}
	switch role {
	case SeriesDataPlane:
		return int(float64(totalMB) * dataPlaneLogShare)
	case SeriesControlPlane:
		if runtimeSplit {
			return int(float64(totalMB) * controlPlaneLogShare)
		}
		return totalMB
	default:
		return totalMB
	}
}

// computeMaxFileSize derives per-file byte cap from series budget and file count.
func computeMaxFileSize(maxFileSizeMB, logFileCount int) int64 {
	if logFileCount < 1 {
		logFileCount = 1
	}
	maxFileSize := int64(maxFileSizeMB) * 1024 * 1024 / int64(logFileCount)
	if maxFileSize < 1024*1024 {
		maxFileSize = 1024 * 1024 // Minimum 1MB
	}
	const maxFileSizeCap = 2 * 1024 * 1024 * 1024 // Maximum 2GB
	if maxFileSize > maxFileSizeCap {
		maxFileSize = maxFileSizeCap
	}
	return maxFileSize
}
