package controlplane

import (
	"errors"
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/models"
)

const (
	HostAPIPort             = 51121
	HostViewerPort          = 80
	DefaultContainerAPIPort = 51121
	DefaultViewerPort       = 8008
	VolumeDBName            = "iofog-controller-db"
	VolumeLogName           = "iofog-controller-log"
	ContainerDBPath         = "/home/runner/.npm-global/lib/node_modules/@datasance/iofogcontroller/src/data/sqlite_files/"
	ContainerLogPath        = "/var/log/iofog-controller"
	ContainerCertMountPath  = "/etc/iofog/controller-cert/"
)

// BuildMicroserviceFromControlPlane converts a validated ControlPlane manifest into
// a runtime microservice model for container launch.
func BuildMicroserviceFromControlPlane(doc *models.ControlPlaneManifest, controllerUUID, image string) (*models.Microservice, error) {
	if doc == nil {
		return nil, errors.New("manifest is nil")
	}
	if strings.TrimSpace(controllerUUID) == "" {
		return nil, errors.New("controllerUUID is required")
	}
	if strings.TrimSpace(image) == "" {
		return nil, errors.New("image is required")
	}

	doc.NormalizeDefaults()
	envMap, err := BuildControllerEnv(doc, controllerUUID)
	if err != nil {
		return nil, err
	}

	ms := models.NewMicroservice(controllerUUID, image)
	ms.MicroserviceName = strings.TrimSpace(doc.Metadata.Name)
	ms.ApplicationName = strings.TrimSpace(doc.Metadata.Namespace)
	ms.IsSystem = true
	ms.IsController = true
	ms.HostNetworkMode = false
	ms.IsPrivileged = false
	ms.RegistryID = 2

	apiPort := DefaultContainerAPIPort
	if doc.Spec.Controller.Port != nil {
		apiPort = *doc.Spec.Controller.Port
	}
	viewerPort := DefaultViewerPort
	if doc.Spec.ECNViewerPort != nil {
		viewerPort = *doc.Spec.ECNViewerPort
	}
	ms.PortMappings = append(ms.PortMappings,
		models.NewPortMapping(HostAPIPort, apiPort, false),
		models.NewPortMapping(HostViewerPort, viewerPort, false),
	)

	ms.CapAdd, ms.CapDrop = mergeControlPlaneCapabilities(nil, nil)

	ms.VolumeMappings = append(ms.VolumeMappings,
		models.NewVolumeMapping(VolumeDBName, ContainerDBPath, "rw", models.VolumeMappingTypeVolume),
		models.NewVolumeMapping(VolumeLogName, ContainerLogPath, "rw", models.VolumeMappingTypeVolume),
	)

	if doc.Spec.HTTPS != nil {
		if path := strings.TrimSpace(doc.Spec.HTTPS.Path); path != "" {
			ms.VolumeMappings = append(ms.VolumeMappings, models.NewVolumeMapping(
				path,
				ContainerCertMountPath,
				"ro",
				models.VolumeMappingTypeBind,
			))
		}
	}

	for key, value := range envMap {
		ms.EnvVars = append(ms.EnvVars, &models.EnvVar{Key: key, Value: value})
	}

	if regID, ok := doc.ControllerRegistryID(); ok {
		ms.RegistryID = regID
	}

	return ms, nil
}

func mergeControlPlaneCapabilities(capAdd, capDrop []string) ([]string, []string) {
	add := append([]string{}, capAdd...)
	drop := make([]string, 0, len(capDrop))
	for _, cap := range capDrop {
		if strings.EqualFold(strings.TrimSpace(cap), "NET_RAW") {
			continue
		}
		drop = append(drop, cap)
	}
	hasNetRaw := false
	for _, cap := range add {
		if strings.EqualFold(strings.TrimSpace(cap), "NET_RAW") {
			hasNetRaw = true
			break
		}
	}
	if !hasNetRaw {
		add = append(add, "NET_RAW")
	}
	return add, drop
}
