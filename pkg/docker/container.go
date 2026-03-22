package docker

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	nat "github.com/docker/go-connections/nat"
	"github.com/eclipse-iofog/agent/internal/config"
	"github.com/eclipse-iofog/agent/internal/models"
	"github.com/eclipse-iofog/agent/internal/utils"
)

// Container represents a Docker container
type Container struct {
	ID     string
	Names  []string
	Image  string
	Status string
	State  string
	Labels map[string]string
}

// GetContainer retrieves a container by microservice UUID
func (c *Client) GetContainer(microserviceUUID string) (*Container, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All:     true,
		Filters: filters.NewArgs(filters.Arg("name", utils.IOFogDockerContainerNamePrefix+microserviceUUID)),
	})

	if err != nil {
		return nil, err
	}

	if len(containers) == 0 {
		return nil, nil
	}

	container := containers[0]
	return &Container{
		ID:     container.ID,
		Names:  container.Names,
		Image:  container.Image,
		Status: container.Status,
		State:  container.State,
		Labels: container.Labels,
	}, nil
}

// GetAllContainers returns all containers regardless of state
func (c *Client) GetAllContainers() ([]Container, error) {
	return c.GetRunningContainers()
}

// GetRunningContainers returns all running containers
func (c *Client) GetRunningContainers() ([]Container, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	containers, err := cli.ContainerList(ctx, container.ListOptions{
		All: true,
	})

	if err != nil {
		return nil, err
	}

	result := make([]Container, 0, len(containers))
	for _, cont := range containers {
		result = append(result, Container{
			ID:     cont.ID,
			Names:  cont.Names,
			Image:  cont.Image,
			Status: cont.Status,
			State:  cont.State,
			Labels: cont.Labels,
		})
	}

	return result, nil
}

// GetContainerStatus retrieves the status of a container
func (c *Client) GetContainerStatus(containerID string) (string, error) {
	cli := c.GetClient()
	if cli == nil {
		return "", fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}

	return inspect.State.Status, nil
}

// IsContainerRunning checks if a container is running
func (c *Client) IsContainerRunning(containerID string) (bool, error) {
	status, err := c.GetContainerStatus(containerID)
	if err != nil {
		return false, err
	}
	return status == "running", nil
}

// StartContainer starts a container
func (c *Client) StartContainer(containerID string) error {
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	return cli.ContainerStart(ctx, containerID, container.StartOptions{})
}

// StopContainer stops a container
func (c *Client) StopContainer(containerID string) error {
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	// Check if container is running first
	running, err := c.IsContainerRunning(containerID)
	if err != nil {
		return err
	}

	if !running {
		return nil // Already stopped
	}

	ctx := c.GetContext()
	return cli.ContainerStop(ctx, containerID, container.StopOptions{})
}

// RemoveContainer removes a container
func (c *Client) RemoveContainer(containerID string, removeVolumes bool) error {
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	return cli.ContainerRemove(ctx, containerID, container.RemoveOptions{
		Force:         true,
		RemoveVolumes: removeVolumes,
	})
}

// GetContainerIPAddress gets the IPv4 address of a container
func (c *Client) GetContainerIPAddress(containerID string) (string, error) {
	cli := c.GetClient()
	if cli == nil {
		return "", fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}

	// Check if container is using host network mode
	if inspect.HostConfig != nil && inspect.HostConfig.NetworkMode == "host" {
		// Return host IP address (will be set by network interface manager)
		// For now, return empty string - this will be handled by the caller
		return "", nil
	}

	// For containers with their own network namespace
	networks := inspect.NetworkSettings.Networks
	if networks != nil {
		// Try bridge network first
		if bridge, ok := networks["bridge"]; ok && bridge.IPAddress != "" {
			return bridge.IPAddress, nil
		}
		// Fallback to first available network
		for _, network := range networks {
			if network.IPAddress != "" {
				return network.IPAddress, nil
			}
		}
	}

	return "", fmt.Errorf("no IP address found for container")
}

// GetContainerStartedAt returns container last start epoch time in milliseconds
func (c *Client) GetContainerStartedAt(containerID string) (int64, error) {
	cli := c.GetClient()
	if cli == nil {
		return 0, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return 0, err
	}

	startedAt := inspect.State.StartedAt
	if startedAt == "" {
		return time.Now().UnixMilli(), nil
	}

	// Parse RFC3339Nano format
	if startTime, err := time.Parse(time.RFC3339Nano, startedAt); err == nil {
		return startTime.UnixMilli(), nil
	}

	// Fallback to RFC3339
	if startTime, err := time.Parse(time.RFC3339, startedAt); err == nil {
		return startTime.UnixMilli(), nil
	}

	return time.Now().UnixMilli(), nil
}

// GetRunningNonIofogContainers returns running containers not managed by ioFog
func (c *Client) GetRunningNonIofogContainers() ([]Container, error) {
	containers, err := c.GetRunningContainers()
	if err != nil {
		return nil, err
	}

	result := make([]Container, 0)
	for _, cont := range containers {
		// Check if container name doesn't start with ioFog prefix
		name := c.GetContainerName(cont)
		if !strings.HasPrefix(name, utils.IOFogDockerContainerNamePrefix) {
			result = append(result, cont)
		}
	}

	return result, nil
}

// GetRouterMicroserviceIP gets the IP address of the router microservice container
func (c *Client) GetRouterMicroserviceIP(routerUUID string) (string, error) {
	if routerUUID == "" {
		return "", nil
	}

	container, err := c.GetContainer(routerUUID)
	if err != nil {
		return "", err
	}

	if container == nil {
		return "", nil
	}

	return c.GetContainerIPAddress(container.ID)
}

// ParseAnnotationsString parses annotations JSON string into a map
func ParseAnnotationsString(annotationsString string) (map[string]string, error) {
	annotationsMap := make(map[string]string)
	if annotationsString == "" {
		return annotationsMap, nil
	}

	// Parse the JSON string
	var jsonMap map[string]interface{}
	if err := json.Unmarshal([]byte(annotationsString), &jsonMap); err != nil {
		return nil, fmt.Errorf("failed to parse annotations JSON: %w", err)
	}

	// Convert to map[string]string
	for key, value := range jsonMap {
		if strValue, ok := value.(string); ok {
			annotationsMap[key] = strValue
		}
	}

	return annotationsMap, nil
}

// GetContainerMicroserviceUUID extracts microservice UUID from container name
func (c *Client) GetContainerMicroserviceUUID(cont Container) string {
	if len(cont.Names) == 0 {
		return ""
	}

	name := cont.Names[0]
	// Remove leading slash if present
	if len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}

	// Remove prefix
	prefix := utils.IOFogDockerContainerNamePrefix
	if len(name) > len(prefix) && name[:len(prefix)] == prefix {
		return name[len(prefix):]
	}

	return name
}

// GetContainerName extracts container name from container
func (c *Client) GetContainerName(cont Container) string {
	if len(cont.Names) == 0 {
		return ""
	}

	name := cont.Names[0]
	// Remove leading slash if present
	if len(name) > 0 && name[0] == '/' {
		return name[1:]
	}
	return name
}

// GetInspectContainersImage gets the image name from container inspection
func (c *Client) GetInspectContainersImage(containerID string) (string, error) {
	cli := c.GetClient()
	if cli == nil {
		return "", fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return "", err
	}

	return inspect.Config.Image, nil
}

// GetMicroserviceStatus gets the microservice status from container inspection
func (c *Client) GetMicroserviceStatus(containerID, _ string) (*models.MicroserviceStatus, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx := c.GetContext()
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, err
	}

	status := models.NewMicroserviceStatus()
	status.ContainerID = containerID

	// Map Docker state to microservice state
	state := inspect.State.Status
	switch state {
	case "running":
		status.Status = models.MicroserviceStateRunning
	case "exited":
		status.Status = models.MicroserviceStateExiting
	case "created":
		status.Status = models.MicroserviceStateCreated
	case "restarting":
		status.Status = models.MicroserviceStateRestarting
	default:
		status.Status = models.MicroserviceStateFromText(state)
	}

	// Get start time
	if inspect.State.StartedAt != "" {
		// Parse time and convert to milliseconds
		// Docker uses RFC3339Nano format: "2006-01-02T15:04:05.999999999Z07:00"
		if startTime, err := time.Parse(time.RFC3339Nano, inspect.State.StartedAt); err == nil {
			status.StartTime = startTime.UnixMilli()
		} else {
			// Fallback: try RFC3339 format
			if startTime, err := time.Parse(time.RFC3339, inspect.State.StartedAt); err == nil {
				status.StartTime = startTime.UnixMilli()
			}
		}
	}

	// Get IP address
	if ip, err := c.GetContainerIPAddress(containerID); err == nil {
		status.IPAddress = &ip
	}

	// Get health status if available (for all container states)
	if inspect.State.Health != nil {
		healthStatus := inspect.State.Health.Status
		status.HealthStatus = &healthStatus
	}

	// Get exec session IDs if container is running
	if state == "running" {
		if len(inspect.ExecIDs) > 0 {
			status.ExecSessionIDs = inspect.ExecIDs
		} else {
			status.ExecSessionIDs = []string{}
		}
	}

	// Note: RestartStuckChecker integration will be handled in ProcessManager
	// to avoid circular dependencies. The checker will be called after getting status
	// to determine if status should be STUCK_IN_RESTART

	return status, nil
}

// AreMicroserviceAndContainerEqual checks if a microservice configuration matches a container
// This matches Java logic: areMicroserviceAndContainerEqual()
func (c *Client) AreMicroserviceAndContainerEqual(containerID string, ms *models.Microservice) bool {
	cli := c.GetClient()
	if cli == nil {
		return false
	}

	ctx := c.GetContext()
	inspect, err := cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return false
	}

	// Check port mappings
	if !c.isPortMappingEqual(inspect, ms) {
		return false
	}

	// Check network mode
	if !c.isNetworkModeEqual(inspect, ms) {
		return false
	}

	// Check environment variables
	if !c.isEnvVarsEqual(inspect, ms) {
		return false
	}

	return true
}

// isPortMappingEqual compares if microservice port mapping is equal to container port mapping
func (c *Client) isPortMappingEqual(inspect types.ContainerJSON, ms *models.Microservice) bool {
	// Get microservice ports
	microservicePorts := c.getMicroservicePorts(ms)

	// Get container ports
	containerPorts := c.getContainerPorts(inspect)

	// Sort both lists for comparison
	sortPortMappings(microservicePorts)
	sortPortMappings(containerPorts)

	// Compare
	if len(microservicePorts) != len(containerPorts) {
		return false
	}

	for i := range microservicePorts {
		if !portMappingsEqual(microservicePorts[i], containerPorts[i]) {
			return false
		}
	}

	return true
}

// isNetworkModeEqual compares if microservice network mode matches container network mode.
// host-network microservice → container must have NetworkMode "host"
// non-host-network microservice → container must be on the "iofog" user-defined bridge
func (c *Client) isNetworkModeEqual(inspect types.ContainerJSON, ms *models.Microservice) bool {
	hostConfig := inspect.HostConfig
	if hostConfig == nil {
		return false
	}

	containerNetworkMode := string(hostConfig.NetworkMode)

	if ms.HostNetworkMode {
		return containerNetworkMode == "host"
	}

	// All non-host-network containers must be on the "iofog" network.
	return containerNetworkMode == "iofog"
}

// isEnvVarsEqual compares if microservice environment variables are equal to container environment variables
func (c *Client) isEnvVarsEqual(inspect types.ContainerJSON, ms *models.Microservice) bool {
	// Get microservice environment variables
	microserviceEnvVars := ms.EnvVars
	if microserviceEnvVars == nil {
		microserviceEnvVars = make([]*models.EnvVar, 0)
	}

	// Get container environment variables from inspect info
	containerEnvArray := inspect.Config.Env
	containerEnvVars := make(map[string]string)
	for _, envVar := range containerEnvArray {
		if envVar != "" {
			parts := strings.SplitN(envVar, "=", 2)
			if len(parts) == 2 {
				containerEnvVars[parts[0]] = parts[1]
			}
		}
	}

	// Check if all microservice env vars exist in container with same values
	for _, microserviceEnvVar := range microserviceEnvVars {
		key := microserviceEnvVar.Key
		expectedValue := microserviceEnvVar.Value

		actualValue, exists := containerEnvVars[key]
		if !exists {
			return false
		}

		if expectedValue != actualValue {
			return false
		}
	}

	return true
}

// getMicroservicePorts gets port mappings from microservice
func (c *Client) getMicroservicePorts(ms *models.Microservice) []*models.PortMapping {
	if ms.PortMappings == nil {
		return make([]*models.PortMapping, 0)
	}
	return ms.PortMappings
}

// getContainerPorts extracts port mappings from container inspect info
func (c *Client) getContainerPorts(inspect types.ContainerJSON) []*models.PortMapping {
	hostConfig := inspect.HostConfig
	if hostConfig == nil || hostConfig.PortBindings == nil {
		return make([]*models.PortMapping, 0)
	}

	portMappings := make([]*models.PortMapping, 0)
	for port, bindings := range hostConfig.PortBindings {
		if len(bindings) == 0 {
			continue
		}

		// Get protocol (tcp or udp)
		isUDP := port.Proto() == "udp"
		insidePort := port.Int()

		// Process each binding
		for _, binding := range bindings {
			if binding.HostPort != "" {
				hostPort, err := strconv.Atoi(binding.HostPort)
				if err == nil {
					portMappings = append(portMappings, &models.PortMapping{
						Outside: hostPort,
						Inside:  insidePort,
						UDP:     isUDP,
					})
				}
			}
		}
	}

	return portMappings
}

// sortPortMappings sorts port mappings by inside port, then outside port
func sortPortMappings(ports []*models.PortMapping) {
	sort.Slice(ports, func(i, j int) bool {
		if ports[i].Inside == ports[j].Inside {
			return ports[i].Outside < ports[j].Outside
		}
		return ports[i].Inside < ports[j].Inside
	})
}

// portMappingsEqual checks if two port mappings are equal
func portMappingsEqual(a, b *models.PortMapping) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Outside == b.Outside && a.Inside == b.Inside && a.UDP == b.UDP
}

// CreateContainer creates a container from microservice configuration
func (c *Client) CreateContainer(ms *models.Microservice, hostName string) (string, error) {
	cli := c.GetClient()
	if cli == nil {
		return "", fmt.Errorf("Docker client not initialized")
	}

	// Ensure the "iofog" bridge network exists before attempting container creation.
	// This guards against races where the network hasn't been created yet (e.g. after
	// a Docker client re-init) — matches Java's synchronous ensureIoFogNetworkExists().
	if !ms.HostNetworkMode {
		if err := c.ensureIoFogNetworkExists(); err != nil {
			return "", fmt.Errorf("failed to ensure iofog network: %w", err)
		}
	}

	ctx := c.GetContext()

	// Get config instance (needed for TZ and other config values)
	cfg := config.GetInstance()

	// Build container config
	// Matches Java: List<String> envVars = new ArrayList<>(Arrays.asList("SELFNAME=" + microservice.getMicroserviceUuid()));
	envVars := []string{fmt.Sprintf("SELFNAME=%s", ms.MicroserviceUUID)}

	// Add user-defined env vars (matching Java: envVars.addAll(microservice.getEnvVars()...))
	if len(ms.EnvVars) > 0 {
		userEnvVars := buildEnvironmentVariables(ms.EnvVars)
		envVars = append(envVars, userEnvVars...)
	}

	// Add TZ if not already present (matching Java: if (envVars.stream().filter(str -> str.trim().contains("TZ")).count() == 0))
	hasTZ := false
	for _, env := range envVars {
		if strings.HasPrefix(strings.TrimSpace(env), "TZ=") {
			hasTZ = true
			break
		}
	}
	if !hasTZ {
		// Add TZ from config (matching Java: envVars.add("TZ=" + Configuration.getTimeZone()))
		timeZone := cfg.TimeZone
		if timeZone == "" {
			timeZone = "UTC" // Default fallback
		}
		envVars = append(envVars, fmt.Sprintf("TZ=%s", timeZone))
	}

	config := &container.Config{
		Image: ms.ImageName,
		Env:   envVars,
		Cmd:   ms.Args,
	}

	// Set hostname
	if hostName != "" {
		config.Hostname = hostName
	}

	// Set user
	if ms.RunAsUser != nil && *ms.RunAsUser != "" {
		config.User = *ms.RunAsUser
	}

	// Platform is set via ContainerCreate options, not Config

	// Set labels
	labels := make(map[string]string)
	labels["iofog-uuid"] = cfg.IOFogUUID
	if ms.IsRouter {
		labels["iofog-router"] = "true"
	}
	if ms.IsNats {
		labels["iofog-nats"] = "true"
	}
	config.Labels = labels

	// Build host config — NetworkMode is set later after ExtraHosts are resolved,
	// matching Java: networkMode("iofog") is only applied when extraHosts is non-empty.
	hostConfig := &container.HostConfig{
		Privileged:      ms.IsPrivileged,
		PublishAllPorts: false,
	}

	// Set runtime if specified
	if ms.Runtime != nil && *ms.Runtime != "" {
		hostConfig.Runtime = *ms.Runtime
	}

	// Set log configuration
	logFiles := 1
	if ms.LogSize > 2 {
		logFiles = int(ms.LogSize / 2)
	}
	hostConfig.LogConfig = container.LogConfig{
		Type: "json-file",
		Config: map[string]string{
			"max-file": fmt.Sprintf("%d", logFiles),
			"max-size": "100m",
		},
	}

	// Build port bindings
	if len(ms.PortMappings) > 0 {
		portBindings, err := buildPortBindings(ms.PortMappings)
		if err != nil {
			return "", err
		}
		hostConfig.PortBindings = portBindings
	}

	// Build volume bindings and mounts
	// Note: We need to handle VOLUME_MOUNT type specially
	if len(ms.VolumeMappings) > 0 {
		binds, mounts, err := buildVolumeBindsAndMounts(ms.VolumeMappings, ms.MicroserviceUUID)
		if err != nil {
			return "", err
		}
		if len(binds) > 0 {
			hostConfig.Binds = binds
		}
		if len(mounts) > 0 {
			hostConfig.Mounts = mounts
		}
	}

	// Build resource limits
	if ms.MemoryLimit != nil {
		hostConfig.Resources.Memory = *ms.MemoryLimit
	}

	// Build security options
	if len(ms.CapAdd) > 0 || len(ms.CapDrop) > 0 {
		hostConfig.CapAdd = ms.CapAdd
		hostConfig.CapDrop = ms.CapDrop
	}

	// Build extra hosts with service.local addition for non-router microservices
	extraHosts := make([]string, 0)
	if len(ms.ExtraHosts) > 0 {
		extraHosts = append(extraHosts, ms.ExtraHosts...)
	}

	// Add service.local host for non-router microservices (matches Java logic)
	if !ms.HostNetworkMode && !ms.IsRouter {
		if !cfg.IsRouterInterior {
			// Get router IP
			routerIP, err := c.GetRouterMicroserviceIP(cfg.RouterUUID)
			if err == nil && routerIP != "" {
				extraHosts = append(extraHosts, fmt.Sprintf("service.local:%s", routerIP))
			}
		} else {
			// Router interior mode - use host parameter
			if hostName != "" {
				extraHosts = append(extraHosts, fmt.Sprintf("service.local:%s", hostName))
			}
		}
	}

	// Filter and validate extra hosts (matches Java validation)
	validHosts := make([]string, 0)
	for _, host := range extraHosts {
		host = strings.TrimSpace(host)
		if host == "" {
			continue
		}
		// Validate format: should be "hostname:ip" with non-empty IP
		colonIndex := strings.Index(host, ":")
		if colonIndex > 0 && colonIndex < len(host)-1 {
			ipPart := strings.TrimSpace(host[colonIndex+1:])
			if ipPart != "" {
				validHosts = append(validHosts, host)
			}
		}
	}
	// Apply network mode + extra hosts — matches Java's conditional logic:
	//   hostNetworkMode → NetworkMode "host"  (no ExtraHosts, no iofog network)
	//   else            → NetworkMode "iofog" (always; ExtraHosts only when non-empty)
	//
	// All non-host-network containers must be on the "iofog" user-defined bridge so that
	// Docker DNS aliases (service discovery) work correctly.  ExtraHosts are independent
	// — they add entries to /etc/hosts and are only set when there are valid entries.
	if ms.HostNetworkMode {
		hostConfig.NetworkMode = container.NetworkMode("host")
	} else {
		hostConfig.NetworkMode = container.NetworkMode("iofog")
		if len(validHosts) > 0 {
			hostConfig.ExtraHosts = validHosts
		}
	}

	// Set PID mode
	if ms.PidMode != nil {
		hostConfig.PidMode = container.PidMode(*ms.PidMode)
	}

	// Set IPC mode
	if ms.IpcMode != nil {
		hostConfig.IpcMode = container.IpcMode(*ms.IpcMode)
	}

	// Set CPU set
	if ms.CPUSetCpus != nil && *ms.CPUSetCpus != "" {
		hostConfig.CpusetCpus = *ms.CPUSetCpus
	}

	// Set CDI devices if specified
	if len(ms.CdiDevs) > 0 {
		deviceRequests := make([]container.DeviceRequest, 0)
		deviceRequest := container.DeviceRequest{
			Driver:    "cdi",
			DeviceIDs: ms.CdiDevs,
		}
		deviceRequests = append(deviceRequests, deviceRequest)
		hostConfig.DeviceRequests = deviceRequests
	}

	// Set annotations if specified
	if ms.Annotations != nil && *ms.Annotations != "" {
		annotations, err := ParseAnnotationsString(*ms.Annotations)
		if err == nil {
			// Docker doesn't have a direct annotations field in HostConfig
			// Annotations are typically set via labels or stored separately
			// For now, we'll add them to labels with a prefix
			if config.Labels == nil {
				config.Labels = make(map[string]string)
			}
			for key, value := range annotations {
				config.Labels["annotation."+key] = value
			}
		}
	}

	// Build health check
	// Note: Healthcheck is set in container.Config, not HostConfig
	if ms.Healthcheck != nil {
		healthConfig := buildHealthCheck(ms.Healthcheck)
		if healthConfig != nil {
			config.Healthcheck = healthConfig
		}
	}

	// Set restart policy
	hostConfig.RestartPolicy = container.RestartPolicy{
		Name:              "always",
		MaximumRetryCount: 0,
	}

	// Container name
	containerName := utils.IOFogDockerContainerNamePrefix + ms.MicroserviceUUID

	// Build networking config with DNS alias for service discovery
	var networkingConfig *network.NetworkingConfig
	if !ms.HostNetworkMode && ms.ApplicationName != "" && ms.MicroserviceName != "" {
		networkingConfig = &network.NetworkingConfig{
			EndpointsConfig: map[string]*network.EndpointSettings{
				"iofog": {
					Aliases: []string{ms.ApplicationName + "." + ms.MicroserviceName},
				},
			},
		}
	}

	// Create container
	createResp, err := cli.ContainerCreate(ctx, config, hostConfig, networkingConfig, nil, containerName)
	if err != nil {
		return "", err
	}

	return createResp.ID, nil
}

// Helper functions for building Docker configurations

// buildEnvironmentVariables converts EnvVar models to Docker env var format
// Matches Java: .map(env -> env.getKey() + "=" + env.getValue())
func buildEnvironmentVariables(envVars []*models.EnvVar) []string {
	if len(envVars) == 0 {
		return nil
	}

	result := make([]string, 0, len(envVars))
	for _, env := range envVars {
		if env.Key != "" || env.Value != "" {
			result = append(result, fmt.Sprintf("%s=%s", env.Key, env.Value))
		}
	}
	return result
}

func buildPortBindings(portMappings []*models.PortMapping) (nat.PortMap, error) {
	if len(portMappings) == 0 {
		return nil, nil
	}

	bindings := make(nat.PortMap)
	for _, pm := range portMappings {
		proto := "tcp"
		if pm.UDP {
			proto = "udp"
		}

		port, err := nat.NewPort(proto, fmt.Sprintf("%d", pm.Inside))
		if err != nil {
			return nil, err
		}

		binding := nat.PortBinding{
			HostIP:   "0.0.0.0",
			HostPort: fmt.Sprintf("%d", pm.Outside),
		}

		if bindings[port] == nil {
			bindings[port] = make([]nat.PortBinding, 0)
		}
		bindings[port] = append(bindings[port], binding)
	}

	return bindings, nil
}

func buildVolumeBindsAndMounts(volumeMappings []*models.VolumeMapping, microserviceUUID string) ([]string, []mount.Mount, error) {
	if len(volumeMappings) == 0 {
		return nil, nil, nil
	}

	binds := make([]string, 0)
	mounts := make([]mount.Mount, 0)

	for _, vm := range volumeMappings {
		// Resolve host destination for volume mounts
		resolvedHostDestination := vm.HostDestination
		if vm.Type == models.VolumeMappingTypeVolumeMount {
			// Use volume mount resolution (will be implemented in volume.go)
			var err error
			resolvedHostDestination, err = ResolveVolumeMountPath(vm.HostDestination, vm.Type, microserviceUUID)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to resolve volume mount path: %w", err)
			}
		}

		// Determine access mode
		isReadOnly := strings.ToLower(vm.AccessMode) == "ro"

		if vm.Type == models.VolumeMappingTypeVolumeMount {
			// Use Mount API for VOLUME_MOUNT type
			m := mount.Mount{
				Type:     mount.TypeBind,
				Source:   resolvedHostDestination,
				Target:   vm.ContainerDestination,
				ReadOnly: isReadOnly,
			}
			mounts = append(mounts, m)
		} else if vm.Type == models.VolumeMappingTypeBind {
			// Use bind mount (legacy format)
			bind := fmt.Sprintf("%s:%s:%s", resolvedHostDestination, vm.ContainerDestination, vm.AccessMode)
			binds = append(binds, bind)
		} else if vm.Type == models.VolumeMappingTypeVolume {
			// Named volume - just add to volumes list (handled separately)
			// For now, we'll use bind mount format
			bind := fmt.Sprintf("%s:%s:%s", resolvedHostDestination, vm.ContainerDestination, vm.AccessMode)
			binds = append(binds, bind)
		}
	}

	return binds, mounts, nil
}

func buildHealthCheck(hc *models.Healthcheck) *container.HealthConfig {
	if hc == nil {
		return nil
	}

	config := &container.HealthConfig{
		Test: hc.Test,
	}

	if hc.Interval != nil {
		// Convert seconds to nanoseconds (Java code uses TimeUnit.SECONDS.toNanos)
		config.Interval = time.Duration(*hc.Interval) * time.Second
	}
	if hc.Timeout != nil {
		// Convert seconds to nanoseconds
		config.Timeout = time.Duration(*hc.Timeout) * time.Second
	}
	if hc.StartPeriod != nil {
		// Convert seconds to nanoseconds
		config.StartPeriod = time.Duration(*hc.StartPeriod) * time.Second
	}
	// Note: StartInterval is not supported in Docker's HealthConfig
	// It's a Docker Compose feature that's not part of the Docker API
	if hc.Retries != nil {
		config.Retries = *hc.Retries
	}

	return config
}
