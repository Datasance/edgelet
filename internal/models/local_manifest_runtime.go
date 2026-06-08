package models

import (
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/workloadmeta"
)

// BuildMicroserviceFromLocalManifest converts a local deploy manifest into
// a runtime microservice model used by the container engine.
func BuildMicroserviceFromLocalManifest(doc *LocalDeployManifest, deploymentID, image string) *Microservice {
	ms := NewMicroservice(deploymentID, image)
	ms.MicroserviceName = doc.Metadata.Name
	ms.ApplicationName = workloadmeta.LocalDeployApplicationName
	ms.Labels = cloneManifestLabels(doc.Metadata.Labels)
	ms.RegistryID = 2
	ms.HostNetworkMode = doc.Spec.Container.HostNetworkMode
	ms.IsPrivileged = doc.Spec.Container.IsPrivileged
	ms.Args = append(ms.Args, doc.Spec.Container.Commands...)
	ms.CapAdd = append(ms.CapAdd, doc.Spec.Container.CapAdd...)
	ms.CapDrop = append(ms.CapDrop, doc.Spec.Container.CapDrop...)
	ms.CdiDevs = append(ms.CdiDevs, doc.Spec.Container.CDIDevices...)
	ms.Schedule = doc.Spec.Schedule
	if strings.TrimSpace(doc.Spec.Container.RunAsUser) != "" {
		runAs := strings.TrimSpace(doc.Spec.Container.RunAsUser)
		ms.RunAsUser = &runAs
	}
	if strings.TrimSpace(doc.Spec.Container.Runtime) != "" {
		runtime := strings.TrimSpace(doc.Spec.Container.Runtime)
		ms.Runtime = &runtime
	}
	if strings.TrimSpace(doc.Spec.Container.Platform) != "" {
		platform := strings.TrimSpace(doc.Spec.Container.Platform)
		ms.Platform = &platform
	}
	if strings.TrimSpace(doc.Spec.Container.IpcMode) != "" {
		ipcMode := strings.TrimSpace(doc.Spec.Container.IpcMode)
		ms.IpcMode = &ipcMode
	}
	if strings.TrimSpace(doc.Spec.Container.PidMode) != "" {
		pidMode := strings.TrimSpace(doc.Spec.Container.PidMode)
		ms.PidMode = &pidMode
	}
	if strings.TrimSpace(doc.Spec.Container.CPUSetCpus) != "" {
		cpuSet := strings.TrimSpace(doc.Spec.Container.CPUSetCpus)
		ms.CPUSetCpus = &cpuSet
	}
	if doc.Spec.Container.MemoryLimit > 0 {
		mem := doc.Spec.Container.MemoryLimit
		ms.MemoryLimit = &mem
	}
	for _, envVar := range doc.Spec.Container.Env {
		ms.EnvVars = append(ms.EnvVars, &EnvVar{Key: envVar.Key, Value: envVar.Value})
	}
	for _, volume := range doc.Spec.Container.Volumes {
		volumeType := strings.ToUpper(strings.TrimSpace(volume.Type))
		if volumeType == "" {
			volumeType = string(VolumeMappingTypeBind)
		}
		ms.VolumeMappings = append(ms.VolumeMappings, &VolumeMapping{
			HostDestination:      volume.HostDestination,
			ContainerDestination: volume.ContainerDestination,
			AccessMode:           volume.AccessMode,
			Type:                 VolumeMappingType(volumeType),
		})
	}
	for _, port := range doc.Spec.Container.Ports {
		ms.PortMappings = append(ms.PortMappings, &PortMapping{
			Inside:  port.Internal,
			Outside: port.External,
			UDP:     strings.EqualFold(strings.TrimSpace(port.Protocol), "udp"),
		})
	}
	for _, host := range doc.Spec.Container.ExtraHosts {
		if strings.TrimSpace(host.Name) == "" || strings.TrimSpace(host.Address) == "" {
			continue
		}
		ms.ExtraHosts = append(ms.ExtraHosts, strings.TrimSpace(host.Name)+":"+strings.TrimSpace(host.Address))
	}
	return ms
}

func cloneManifestLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(v)
	}
	return out
}
