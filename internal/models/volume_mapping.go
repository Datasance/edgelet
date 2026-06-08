package models

// VolumeMapping represents microservice volume mappings for Docker run options
type VolumeMapping struct {
	HostDestination      string            `json:"hostDestination" yaml:"hostDestination"`
	ContainerDestination string            `json:"containerDestination" yaml:"containerDestination"`
	AccessMode           string            `json:"accessMode" yaml:"accessMode"`
	Type                 VolumeMappingType `json:"type" yaml:"type"`
}

// NewVolumeMapping creates a new VolumeMapping
func NewVolumeMapping(hostDestination, containerDestination, accessMode string, volumeType VolumeMappingType) *VolumeMapping {
	return &VolumeMapping{
		HostDestination:      hostDestination,
		ContainerDestination: containerDestination,
		AccessMode:           accessMode,
		Type:                 volumeType,
	}
}

// Equals checks if two VolumeMappings are equal
func (v *VolumeMapping) Equals(other *VolumeMapping) bool {
	if other == nil {
		return false
	}
	return v.HostDestination == other.HostDestination &&
		v.ContainerDestination == other.ContainerDestination &&
		v.AccessMode == other.AccessMode &&
		v.Type == other.Type
}
