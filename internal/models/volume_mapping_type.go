package models

// VolumeMappingType represents the type of volume mapping
type VolumeMappingType string

const (
	VolumeMappingTypeVolume     VolumeMappingType = "VOLUME"
	VolumeMappingTypeBind       VolumeMappingType = "BIND"
	VolumeMappingTypeVolumeMount VolumeMappingType = "VOLUME_MOUNT"
)
