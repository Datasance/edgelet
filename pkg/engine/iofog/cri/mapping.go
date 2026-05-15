//go:build linux

package cri

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/utils"
	"github.com/eclipse-iofog/agent/internal/volumemount"
	"github.com/eclipse-iofog/agent/internal/workloadmeta"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// Runtime handler names — must match containerd config.
const (
	RuntimeHandlerRunc      = "runc"
	RuntimeHandlerRuncLocal = "runc-local"
	RuntimeHandlerSpin      = "spin"
	RuntimeHandlerSpinLocal = "spin-local"
)

// linuxNamespaceOptionsFromMicroservice returns namespace options for both sandbox and
// container from the same Microservice fields (single source of truth).
func linuxNamespaceOptionsFromMicroservice(ms *models.Microservice) *runtimeapi.NamespaceOption {
	network := runtimeapi.NamespaceMode_POD
	if ms.HostNetworkMode {
		network = runtimeapi.NamespaceMode_NODE
	}
	pid := runtimeapi.NamespaceMode_CONTAINER
	if ms.PidMode != nil && strings.TrimSpace(*ms.PidMode) == "host" {
		pid = runtimeapi.NamespaceMode_NODE
	}
	ipc := runtimeapi.NamespaceMode_CONTAINER
	if ms.IpcMode != nil && strings.TrimSpace(*ms.IpcMode) == "host" {
		ipc = runtimeapi.NamespaceMode_NODE
	}
	return &runtimeapi.NamespaceOption{
		Network: network,
		Pid:     pid,
		Ipc:     ipc,
	}
}

func podSandboxNeedsLinuxBlock(ms *models.Microservice) bool {
	if ms == nil {
		return false
	}
	if ms.IsPrivileged || ms.HostNetworkMode {
		return true
	}
	if ms.PidMode != nil && strings.TrimSpace(*ms.PidMode) == "host" {
		return true
	}
	if ms.IpcMode != nil && strings.TrimSpace(*ms.IpcMode) == "host" {
		return true
	}
	return false
}

// PodSandboxConfigFromMicroservice builds a CRI PodSandboxConfig from a
// microservice. 1 microservice = 1 pod sandbox.
func PodSandboxConfigFromMicroservice(ms *models.Microservice, hostname, logDir, nodeUID string) *runtimeapi.PodSandboxConfig {
	containerName := utils.IOFogDockerContainerNamePrefix + ms.MicroserviceUUID
	metadata := &runtimeapi.PodSandboxMetadata{
		Name:      containerName,
		Namespace: constants.IofogContainerdNamespace,
		Uid:       ms.MicroserviceUUID,
		Attempt:   0,
	}

	portMappings := buildCRIPortMappings(ms.PortMappings)
	labels := workloadmeta.BuildLabels(workloadmeta.BuildInput{
		MicroserviceUUID: ms.MicroserviceUUID,
		MicroserviceName: ms.MicroserviceName,
		ApplicationName:  ms.ApplicationName,
		NodeUUID:         nodeUID,
		RuntimeEngine:    workloadmeta.RuntimeEngineIofog,
		IsRouter:         ms.IsRouter,
		IsNats:           ms.IsNats,
		HostNetwork:      ms.HostNetworkMode,
		IsSystem:         false,
		UserLabels:       ms.Labels,
	})
	annotations := map[string]string{
		"iofog.network": constants.IofogNetworkName,
	}
	if isLocalWorkload(ms) && !ms.HostNetworkMode {
		annotations["iofog.network"] = constants.IofogLocalNetworkName
	}

	// Hostname: must be empty when using host network (NODE) — CRI spec and runc
	// require it ("unable to set hostname without a private UTS namespace").
	// For non-host-network pods, use container name as hostname.
	sandboxHostname := ""
	if !ms.HostNetworkMode {
		sandboxHostname = containerName
	}
	config := &runtimeapi.PodSandboxConfig{
		Metadata:     metadata,
		Hostname:     sandboxHostname,
		LogDirectory: logDir,
		PortMappings: portMappings,
		Labels:       labels,
		Annotations:  annotations,
	}

	if podSandboxNeedsLinuxBlock(ms) {
		config.Linux = &runtimeapi.LinuxPodSandboxConfig{
			SecurityContext: &runtimeapi.LinuxSandboxSecurityContext{
				Privileged:       ms.IsPrivileged,
				NamespaceOptions: linuxNamespaceOptionsFromMicroservice(ms),
			},
		}
	}

	return config
}

// ContainerConfigFromMicroservice builds a CRI ContainerConfig from a
// microservice. Includes image, env, args, mounts, and security context.
// sandboxID is stored in labels for teardown and recovery.
func ContainerConfigFromMicroservice(ms *models.Microservice, hostname string, envVars []string, logPath string, hostsFilePath string, resolvFilePath string, sandboxID string, nodeUID string) (*runtimeapi.ContainerConfig, error) {
	containerName := utils.IOFogDockerContainerNamePrefix + ms.MicroserviceUUID
	metadata := &runtimeapi.ContainerMetadata{
		Name:    containerName,
		Attempt: 0,
	}

	image := &runtimeapi.ImageSpec{Image: ms.ImageName}

	envs := make([]*runtimeapi.KeyValue, 0, len(envVars))
	for _, e := range envVars {
		// envVars are "KEY=VALUE".
		for i := 0; i < len(e); i++ {
			if e[i] == '=' {
				envs = append(envs, &runtimeapi.KeyValue{Key: e[:i], Value: e[i+1:]})
				break
			}
		}
	}

	mounts, err := buildCRIMounts(ms, hostsFilePath, resolvFilePath)
	if err != nil {
		return nil, err
	}

	labels := workloadmeta.BuildLabels(workloadmeta.BuildInput{
		MicroserviceUUID: ms.MicroserviceUUID,
		MicroserviceName: ms.MicroserviceName,
		ApplicationName:  ms.ApplicationName,
		NodeUUID:         nodeUID,
		RuntimeEngine:    workloadmeta.RuntimeEngineIofog,
		IsRouter:         ms.IsRouter,
		IsNats:           ms.IsNats,
		HostNetwork:      ms.HostNetworkMode,
		IsSystem:         false,
		SandboxID:        sandboxID,
		UserLabels:       ms.Labels,
	})

	secCtx := &runtimeapi.LinuxContainerSecurityContext{
		Privileged: ms.IsPrivileged,
	}

	// CapAdd / CapDrop
	if len(ms.CapAdd) > 0 || len(ms.CapDrop) > 0 {
		secCtx.Capabilities = &runtimeapi.Capability{
			AddCapabilities:  ms.CapAdd,
			DropCapabilities: ms.CapDrop,
		}
	}

	// RunAsUser (uid) or RunAsUsername
	if ms.RunAsUser != nil && *ms.RunAsUser != "" {
		s := strings.TrimSpace(*ms.RunAsUser)
		if uid, err := strconv.ParseInt(s, 10, 64); err == nil {
			secCtx.RunAsUser = &runtimeapi.Int64Value{Value: uid}
		} else {
			secCtx.RunAsUsername = s
		}
	}

	secCtx.NamespaceOptions = linuxNamespaceOptionsFromMicroservice(ms)

	// Resources: CPU set, memory limit
	resources := &runtimeapi.LinuxContainerResources{}
	if ms.CPUSetCpus != nil && *ms.CPUSetCpus != "" {
		resources.CpusetCpus = *ms.CPUSetCpus
	}
	if ms.MemoryLimit != nil && *ms.MemoryLimit > 0 {
		resources.MemoryLimitInBytes = *ms.MemoryLimit
	}

	// Annotations
	annotations := make(map[string]string)
	if ms.Annotations != nil && *ms.Annotations != "" {
		var ann map[string]string
		if err := json.Unmarshal([]byte(*ms.Annotations), &ann); err == nil {
			for k, v := range ann {
				annotations[k] = v
			}
		}
	}

	// CDI devices
	var cdiDevices []*runtimeapi.CDIDevice
	for _, d := range ms.CdiDevs {
		d = strings.TrimSpace(d)
		if d != "" {
			cdiDevices = append(cdiDevices, &runtimeapi.CDIDevice{Name: d})
		}
	}

	config := &runtimeapi.ContainerConfig{
		Metadata:    metadata,
		Image:       image,
		Args:        ms.Args,
		Envs:        envs,
		Mounts:      mounts,
		Labels:      labels,
		Annotations: annotations,
		LogPath:     logPath,
		CDIDevices:  cdiDevices,
		Linux: &runtimeapi.LinuxContainerConfig{
			SecurityContext: secCtx,
			Resources:       resources,
		},
	}

	return config, nil
}

func buildCRIPortMappings(ports []*models.PortMapping) []*runtimeapi.PortMapping {
	out := make([]*runtimeapi.PortMapping, 0, len(ports))
	for _, p := range ports {
		if p.Outside <= 0 {
			// Skip dynamic/invalid host ports — CRI and CNI portmap ignore HostPort <= 0.
			continue
		}
		proto := runtimeapi.Protocol_TCP
		if p.UDP {
			proto = runtimeapi.Protocol_UDP
		}
		out = append(out, &runtimeapi.PortMapping{
			HostPort:      int32(p.Outside),
			ContainerPort: int32(p.Inside),
			Protocol:      proto,
		})
	}
	return out
}

func buildCRIMounts(ms *models.Microservice, hostsFilePath string, resolvFilePath string) ([]*runtimeapi.Mount, error) {
	vmm := volumemount.GetInstance()
	var mounts []*runtimeapi.Mount

	for _, vm := range ms.VolumeMappings {
		isVolumeMount := vm.Type == models.VolumeMappingTypeVolumeMount
		source, err := vmm.ResolveHostPath(ms.MicroserviceUUID, vm.HostDestination, isVolumeMount, ms.RunAsUser)
		if err != nil {
			return nil, err
		}
		readOnly := vm.AccessMode == "ro"
		mounts = append(mounts, &runtimeapi.Mount{
			ContainerPath: vm.ContainerDestination,
			HostPath:      source,
			Readonly:      readOnly,
		})
	}

	if hostsFilePath != "" {
		mounts = append(mounts, &runtimeapi.Mount{
			ContainerPath: "/etc/hosts",
			HostPath:      hostsFilePath,
			Readonly:      false,
		})
	}

	if !ms.HostNetworkMode {
		hostResolvPath := "/etc/resolv.conf"
		if strings.TrimSpace(resolvFilePath) != "" {
			hostResolvPath = strings.TrimSpace(resolvFilePath)
		}
		mounts = append(mounts, &runtimeapi.Mount{
			ContainerPath: "/etc/resolv.conf",
			HostPath:      hostResolvPath,
			Readonly:      true,
		})
	}

	return mounts, nil
}

// GetRuntimeHandler returns the CRI runtime handler for the microservice.
// Privileged and host-namespace workloads use runc, not spin.
func GetRuntimeHandler(ms *models.Microservice) string {
	if ms == nil {
		return RuntimeHandlerRunc
	}
	local := isLocalWorkload(ms) && !ms.HostNetworkMode
	needsRunc := ms.IsPrivileged || ms.HostNetworkMode ||
		(ms.PidMode != nil && strings.TrimSpace(*ms.PidMode) == "host") ||
		(ms.IpcMode != nil && strings.TrimSpace(*ms.IpcMode) == "host")
	if needsRunc {
		return RuntimeHandlerRunc
	}
	if ms.Runtime != nil && *ms.Runtime == "spin" {
		if local {
			return RuntimeHandlerSpinLocal
		}
		return RuntimeHandlerSpin
	}
	if local {
		return RuntimeHandlerRuncLocal
	}
	return RuntimeHandlerRunc
}

func isLocalWorkload(ms *models.Microservice) bool {
	if ms == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(ms.ApplicationName), "local")
}

// LogPathsForCRI returns the log directory and log path for the container.
// CRI expects log_directory on the pod sandbox and log_path on the container.
// Uses "0.log" (Kubernetes convention) — containerd CRI writes both stdout and stderr
// to this single file with CRI log format (timestamp stream flag message).
func LogPathsForCRI(logDir, microserviceUUID string) (logDirectory, logPath string) {
	logDirectory = filepath.Join(logDir, microserviceUUID)
	logPath = "0.log"
	return logDirectory, logPath
}
