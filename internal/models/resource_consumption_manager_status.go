package models

// ResourceConsumptionManagerStatus represents the Resource Consumption Manager status
type ResourceConsumptionManagerStatus struct {
	MemoryUsage     float64 `json:"memoryUsage" yaml:"memoryUsage"`         // Memory usage in MiB
	DiskUsage       float64 `json:"diskUsage" yaml:"diskUsage"`             // Disk usage in GiB
	CPUUsage        float64 `json:"cpuUsage" yaml:"cpuUsage"`               // CPU usage percentage (0-100)
	MemoryViolation bool    `json:"memoryViolation" yaml:"memoryViolation"` // Whether memory limit is violated
	DiskViolation   bool    `json:"diskViolation" yaml:"diskViolation"`     // Whether disk limit is violated
	CPUViolation    bool    `json:"cpuViolation" yaml:"cpuViolation"`       // Whether CPU limit is violated
	AvailableMemory int64   `json:"availableMemory" yaml:"availableMemory"` // System available memory in bytes
	TotalCPU        float64 `json:"totalCpu" yaml:"totalCpu"`               // Total system CPU usage percentage
	AvailableDisk   int64   `json:"availableDisk" yaml:"availableDisk"`     // System available disk space in bytes
	TotalDiskSpace  int64   `json:"totalDiskSpace" yaml:"totalDiskSpace"`   // Total system disk space in bytes
}

// NewResourceConsumptionManagerStatus creates a new ResourceConsumptionManagerStatus
func NewResourceConsumptionManagerStatus() *ResourceConsumptionManagerStatus {
	return &ResourceConsumptionManagerStatus{}
}

// SetMemoryUsage sets the memory usage and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetMemoryUsage(memoryUsage float64) *ResourceConsumptionManagerStatus {
	r.MemoryUsage = memoryUsage
	return r
}

// SetDiskUsage sets the disk usage and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetDiskUsage(diskUsage float64) *ResourceConsumptionManagerStatus {
	r.DiskUsage = diskUsage
	return r
}

// SetCPUUsage sets the CPU usage and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetCPUUsage(cpuUsage float64) *ResourceConsumptionManagerStatus {
	r.CPUUsage = cpuUsage
	return r
}

// SetMemoryViolation sets the memory violation flag and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetMemoryViolation(violation bool) *ResourceConsumptionManagerStatus {
	r.MemoryViolation = violation
	return r
}

// SetDiskViolation sets the disk violation flag and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetDiskViolation(violation bool) *ResourceConsumptionManagerStatus {
	r.DiskViolation = violation
	return r
}

// SetCPUViolation sets the CPU violation flag and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetCPUViolation(violation bool) *ResourceConsumptionManagerStatus {
	r.CPUViolation = violation
	return r
}

// SetAvailableMemory sets the available memory and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetAvailableMemory(availableMemory int64) *ResourceConsumptionManagerStatus {
	r.AvailableMemory = availableMemory
	return r
}

// SetTotalCPU sets the total CPU usage and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetTotalCPU(totalCPU float64) *ResourceConsumptionManagerStatus {
	r.TotalCPU = totalCPU
	return r
}

// SetAvailableDisk sets the available disk space and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetAvailableDisk(availableDisk int64) *ResourceConsumptionManagerStatus {
	r.AvailableDisk = availableDisk
	return r
}

// SetTotalDiskSpace sets the total disk space and returns the status for chaining
func (r *ResourceConsumptionManagerStatus) SetTotalDiskSpace(totalDiskSpace int64) *ResourceConsumptionManagerStatus {
	r.TotalDiskSpace = totalDiskSpace
	return r
}
