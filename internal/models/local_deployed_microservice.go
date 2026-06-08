package models

import (
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

// LocalDeployedMicroservice represents a microservice deployed by EdgeletAPI/CLI (not controller-managed).
type LocalDeployedMicroservice struct {
	LocalUUID          string
	ApplicationName    string
	MicroserviceName   string
	SourceName         string
	ManifestYAML       string
	ImageName          string
	State              string
	ContainerID        string
	DesiredState       string
	RuntimeState       string
	LastError          string
	RestartCount       int
	LastTransitionAt   int64
	LastReconcileAt    int64
	LastStartAttemptAt int64
	FailureCount       int
	DeletedAt          *int64
	Generation         int64
	ObservedGeneration int64
}

// NormalizeDefaults ensures lifecycle defaults are present for local records.
func (m *LocalDeployedMicroservice) NormalizeDefaults() {
	if m == nil {
		return
	}
	if strings.TrimSpace(m.ApplicationName) == "" {
		m.ApplicationName = workloadmeta.LocalDeployApplicationName
	}
	if strings.TrimSpace(m.DesiredState) == "" {
		m.DesiredState = "running"
	}
	if strings.TrimSpace(m.RuntimeState) == "" {
		if strings.TrimSpace(m.State) != "" {
			m.RuntimeState = strings.TrimSpace(m.State)
		} else {
			m.RuntimeState = "unknown"
		}
	}
	if strings.TrimSpace(m.State) == "" {
		m.State = m.RuntimeState
	}
	if m.LastTransitionAt <= 0 {
		m.LastTransitionAt = time.Now().Unix()
	}
	if m.LastReconcileAt < 0 {
		m.LastReconcileAt = 0
	}
	if m.LastStartAttemptAt < 0 {
		m.LastStartAttemptAt = 0
	}
	if m.FailureCount < 0 {
		m.FailureCount = 0
	}
	if m.Generation <= 0 {
		m.Generation = 1
	}
	if m.ObservedGeneration < 0 {
		m.ObservedGeneration = 0
	}
}
