package models

import (
	"strings"
	"time"
)

const ControlPlaneDefaultNamespace = "default"

// ControlPlaneDeployment is the singleton SQLite record for a local ControlPlane manifest.
type ControlPlaneDeployment struct {
	ControllerUUID     string
	Namespace          string
	Name               string
	ManifestYAML       string
	Image              string
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

// NormalizeDefaults ensures lifecycle defaults for control plane records.
func (d *ControlPlaneDeployment) NormalizeDefaults() {
	if d == nil {
		return
	}
	if strings.TrimSpace(d.Namespace) == "" {
		d.Namespace = ControlPlaneDefaultNamespace
	}
	if strings.TrimSpace(d.DesiredState) == "" {
		d.DesiredState = "running"
	}
	if strings.TrimSpace(d.RuntimeState) == "" {
		if strings.TrimSpace(d.State) != "" {
			d.RuntimeState = strings.TrimSpace(d.State)
		} else {
			d.RuntimeState = "unknown"
		}
	}
	if strings.TrimSpace(d.State) == "" {
		d.State = d.RuntimeState
	}
	if d.LastTransitionAt <= 0 {
		d.LastTransitionAt = time.Now().Unix()
	}
	if d.LastReconcileAt < 0 {
		d.LastReconcileAt = 0
	}
	if d.LastStartAttemptAt < 0 {
		d.LastStartAttemptAt = 0
	}
	if d.FailureCount < 0 {
		d.FailureCount = 0
	}
	if d.Generation <= 0 {
		d.Generation = 1
	}
	if d.ObservedGeneration < 0 {
		d.ObservedGeneration = 0
	}
}
