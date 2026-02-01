package models

// VolumeMountManagerStatus represents the Volume Mount Manager status
type VolumeMountManagerStatus struct {
	ActiveMounts int64 `json:"activeMounts" yaml:"activeMounts"` // Number of active volume mounts
	LastUpdate   int64 `json:"lastUpdate" yaml:"lastUpdate"`       // Timestamp of last update
}

// NewVolumeMountManagerStatus creates a new VolumeMountManagerStatus
func NewVolumeMountManagerStatus() *VolumeMountManagerStatus {
	return &VolumeMountManagerStatus{}
}

// SetActiveMounts sets the active mounts count and returns the status for chaining
func (v *VolumeMountManagerStatus) SetActiveMounts(count int64) *VolumeMountManagerStatus {
	v.ActiveMounts = count
	return v
}

// SetLastUpdate sets the last update time and returns the status for chaining
func (v *VolumeMountManagerStatus) SetLastUpdate(timestamp int64) *VolumeMountManagerStatus {
	v.LastUpdate = timestamp
	return v
}
