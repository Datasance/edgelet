package models

import (
	"sync"
)

// DeleteLock is a global lock for microservice deletion operations
var DeleteLock = &sync.Mutex{}

// Microservice represents a microservice/container configuration
type Microservice struct {
	// Required fields
	MicroserviceUUID string `json:"microserviceUuid" yaml:"microserviceUuid"`
	ImageName        string `json:"imageName" yaml:"imageName"`

	// Optional fields
	PortMappings      []*PortMapping  `json:"portMappings,omitempty" yaml:"portMappings,omitempty"`
	Config            *string          `json:"config,omitempty" yaml:"config,omitempty"`
	RunAsUser         *string          `json:"runAsUser,omitempty" yaml:"runAsUser,omitempty"`
	Platform          *string          `json:"platform,omitempty" yaml:"platform,omitempty"`
	Runtime           *string          `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Routes            []string         `json:"routes,omitempty" yaml:"routes,omitempty"`
	ContainerID       string           `json:"containerId" yaml:"containerId"`
	RegistryID        int              `json:"registryId" yaml:"registryId"`
	ContainerIPAddress *string          `json:"containerIpAddress,omitempty" yaml:"containerIpAddress,omitempty"`
	Rebuild           bool             `json:"rebuild" yaml:"rebuild"`
	HostNetworkMode   bool             `json:"hostNetworkMode" yaml:"hostNetworkMode"`
	IsPrivileged      bool             `json:"isPrivileged" yaml:"isPrivileged"`
	LogSize           int64            `json:"logSize" yaml:"logSize"`
	VolumeMappings    []*VolumeMapping `json:"volumeMappings,omitempty" yaml:"volumeMappings,omitempty"`
	IsUpdating        bool             `json:"isUpdating" yaml:"isUpdating"`
	EnvVars           []*EnvVar        `json:"envVars,omitempty" yaml:"envVars,omitempty"`
	Args              []string         `json:"args,omitempty" yaml:"args,omitempty"`
	CdiDevs           []string         `json:"cdiDevs,omitempty" yaml:"cdiDevs,omitempty"`
	Annotations       *string          `json:"annotations,omitempty" yaml:"annotations,omitempty"`
	CapAdd            []string         `json:"capAdd,omitempty" yaml:"capAdd,omitempty"`
	CapDrop           []string         `json:"capDrop,omitempty" yaml:"capDrop,omitempty"`
	ExtraHosts        []string         `json:"extraHosts,omitempty" yaml:"extraHosts,omitempty"`
	IsConsumer        bool             `json:"isConsumer" yaml:"isConsumer"`
	IsRouter          bool             `json:"isRouter" yaml:"isRouter"`
	PidMode           *string          `json:"pidMode,omitempty" yaml:"pidMode,omitempty"`
	IpcMode           *string          `json:"ipcMode,omitempty" yaml:"ipcMode,omitempty"`
	ExecEnabled       bool             `json:"execEnabled" yaml:"execEnabled"`
	Schedule          int              `json:"schedule" yaml:"schedule"`
	CpuSetCpus        *string          `json:"cpuSetCpus,omitempty" yaml:"cpuSetCpus,omitempty"`
	MemoryLimit       *int64           `json:"memoryLimit,omitempty" yaml:"memoryLimit,omitempty"` // in bytes

	// Internal state fields
	Delete            bool       `json:"delete" yaml:"delete"`
	DeleteWithCleanup bool       `json:"deleteWithCleanup" yaml:"deleteWithCleanup"`
	IsStuckInRestart  bool       `json:"isStuckInRestart" yaml:"isStuckInRestart"`
	Healthcheck       *Healthcheck `json:"healthcheck,omitempty" yaml:"healthcheck,omitempty"`

	// Mutex for thread-safe access to IsUpdating
	mu sync.RWMutex
}

// NewMicroservice creates a new Microservice with required fields
func NewMicroservice(microserviceUUID, imageName string) *Microservice {
	return &Microservice{
		MicroserviceUUID: microserviceUUID,
		ImageName:        imageName,
		ContainerID:      "",
		PortMappings:      make([]*PortMapping, 0),
		Routes:            make([]string, 0),
		VolumeMappings:    make([]*VolumeMapping, 0),
		EnvVars:           make([]*EnvVar, 0),
		Args:              make([]string, 0),
		CdiDevs:           make([]string, 0),
		CapAdd:            make([]string, 0),
		CapDrop:           make([]string, 0),
		ExtraHosts:        make([]string, 0),
	}
}

// IsUpdating returns whether the microservice is currently updating (thread-safe)
func (m *Microservice) GetIsUpdating() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.IsUpdating
}

// SetIsUpdating sets the updating state (thread-safe)
func (m *Microservice) SetIsUpdating(updating bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.IsUpdating = updating
}

// GetMemoryLimitMB returns the memory limit in MB
func (m *Microservice) GetMemoryLimitMB() *int64 {
	if m.MemoryLimit == nil {
		return nil
	}
	mb := *m.MemoryLimit / (1024 * 1024)
	return &mb
}

// SetMemoryLimitMB sets the memory limit from MB to bytes
func (m *Microservice) SetMemoryLimitMB(memoryLimitMB *int64) {
	if memoryLimitMB == nil {
		m.MemoryLimit = nil
		return
	}
	bytes := *memoryLimitMB * 1024 * 1024
	m.MemoryLimit = &bytes
}

// Equals checks if two Microservices are equal based on UUID
func (m *Microservice) Equals(other *Microservice) bool {
	if other == nil {
		return false
	}
	return m.MicroserviceUUID == other.MicroserviceUUID
}

// Validate validates the microservice configuration
func (m *Microservice) Validate() error {
	if m.MicroserviceUUID == "" {
		return &ValidationError{Field: "microserviceUuid", Message: "microserviceUuid is required"}
	}
	if m.ImageName == "" {
		return &ValidationError{Field: "imageName", Message: "imageName is required"}
	}
	return nil
}
