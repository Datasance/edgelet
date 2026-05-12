package models

// LocalDeployedMicroservice represents a microservice deployed by LocalAPI/CLI (not controller-managed).
type LocalDeployedMicroservice struct {
	LocalUUID       string
	ApplicationName string
	MicroserviceName string
	SourceName      string
	ManifestYAML    string
	ImageName       string
	State           string
	ContainerID     string
}
