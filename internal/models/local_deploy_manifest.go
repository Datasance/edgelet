package models

import (
	"fmt"
	"strings"
)

// LocalDeployManifest mirrors single-microservice deploy shape used by controller deploy flows.
type LocalDeployManifest struct {
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	Kind       string `yaml:"kind" json:"kind"`
	Metadata   struct {
		Name      string            `yaml:"name" json:"name"`
		Namespace string            `yaml:"namespace,omitempty" json:"namespace,omitempty"`
		Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	} `yaml:"metadata" json:"metadata"`
	Spec struct {
		Images struct {
			X86      string `yaml:"x86,omitempty" json:"x86,omitempty"`
			Arm      string `yaml:"arm,omitempty" json:"arm,omitempty"`
			Registry *int   `yaml:"registry,omitempty" json:"registry,omitempty"`
		} `yaml:"images" json:"images"`
		Container struct {
			Annotations     map[string]interface{} `yaml:"annotations,omitempty" json:"annotations,omitempty"`
			HostNetworkMode bool                   `yaml:"hostNetworkMode" json:"hostNetworkMode"`
			IsPrivileged    bool                   `yaml:"isPrivileged" json:"isPrivileged"`
			CapAdd          []string               `yaml:"capAdd,omitempty" json:"capAdd,omitempty"`
			CapDrop         []string               `yaml:"capDrop,omitempty" json:"capDrop,omitempty"`
			IpcMode         string                 `yaml:"ipcMode,omitempty" json:"ipcMode,omitempty"`
			PidMode         string                 `yaml:"pidMode,omitempty" json:"pidMode,omitempty"`
			RunAsUser       string                 `yaml:"runAsUser,omitempty" json:"runAsUser,omitempty"`
			Platform        string                 `yaml:"platform,omitempty" json:"platform,omitempty"`
			Runtime         string                 `yaml:"runtime,omitempty" json:"runtime,omitempty"`
			CDIDevices      []string               `yaml:"cdiDevices,omitempty" json:"cdiDevices,omitempty"`
			Volumes         []struct {
				HostDestination      string `yaml:"hostDestination" json:"hostDestination"`
				ContainerDestination string `yaml:"containerDestination" json:"containerDestination"`
				AccessMode           string `yaml:"accessMode,omitempty" json:"accessMode,omitempty"`
				Type                 string `yaml:"type,omitempty" json:"type,omitempty"`
			} `yaml:"volumes,omitempty" json:"volumes,omitempty"`
			ExtraHosts []struct {
				Name    string `yaml:"name" json:"name"`
				Address string `yaml:"address" json:"address"`
			} `yaml:"extraHosts,omitempty" json:"extraHosts,omitempty"`
			Env []struct {
				Key   string `yaml:"key" json:"key"`
				Value string `yaml:"value" json:"value"`
			} `yaml:"env,omitempty" json:"env,omitempty"`
			Ports []struct {
				Internal int    `yaml:"internal" json:"internal"`
				External int    `yaml:"external" json:"external"`
				Protocol string `yaml:"protocol,omitempty" json:"protocol,omitempty"`
			} `yaml:"ports,omitempty" json:"ports,omitempty"`
			Commands    []string `yaml:"commands,omitempty" json:"commands,omitempty"`
			CPUSetCpus  string   `yaml:"cpuSetCpus,omitempty" json:"cpuSetCpus,omitempty"`
			MemoryLimit int64    `yaml:"memoryLimit,omitempty" json:"memoryLimit,omitempty"`
			HealthCheck struct {
				Test        []string `yaml:"test,omitempty" json:"test,omitempty"`
				Interval    int      `yaml:"interval,omitempty" json:"interval,omitempty"`
				Timeout     int      `yaml:"timeout,omitempty" json:"timeout,omitempty"`
				StartPeriod int      `yaml:"startPeriod,omitempty" json:"startPeriod,omitempty"`
				Retries     int      `yaml:"retries,omitempty" json:"retries,omitempty"`
			} `yaml:"healthCheck,omitempty" json:"healthCheck,omitempty"`
		} `yaml:"container,omitempty" json:"container,omitempty"`
		Schedule int                    `yaml:"schedule,omitempty" json:"schedule,omitempty"`
		Config   map[string]interface{} `yaml:"config,omitempty" json:"config,omitempty"`
	} `yaml:"spec" json:"spec"`
}

func (m *LocalDeployManifest) Validate() error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if strings.TrimSpace(m.APIVersion) == "" {
		return fmt.Errorf("apiVersion is required")
	}
	if strings.TrimSpace(m.Kind) == "" {
		return fmt.Errorf("kind is required")
	}
	if strings.TrimSpace(m.Kind) != "Microservice" {
		return fmt.Errorf("kind must be Microservice")
	}
	switch strings.TrimSpace(m.APIVersion) {
	case "datasance.com/v3", "iofog.org/v3":
	default:
		return fmt.Errorf("apiVersion must be datasance.com/v3 or iofog.org/v3")
	}
	if strings.TrimSpace(m.Metadata.Name) == "" {
		return fmt.Errorf("metadata.name is required")
	}
	if strings.TrimSpace(m.Spec.Images.X86) == "" && strings.TrimSpace(m.Spec.Images.Arm) == "" {
		return fmt.Errorf("spec.images.x86 or spec.images.arm is required")
	}
	return nil
}

func (m *LocalDeployManifest) ResolveImageForArch(arch string) string {
	normalized := strings.TrimSpace(strings.ToLower(arch))
	if normalized == "arm" && strings.TrimSpace(m.Spec.Images.Arm) != "" {
		return strings.TrimSpace(m.Spec.Images.Arm)
	}
	if strings.TrimSpace(m.Spec.Images.X86) != "" {
		return strings.TrimSpace(m.Spec.Images.X86)
	}
	return strings.TrimSpace(m.Spec.Images.Arm)
}
