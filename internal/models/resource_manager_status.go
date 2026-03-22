package models

// ResourceManagerStatus represents the Resource Manager status
type ResourceManagerStatus struct {
	HWInfo             string `json:"hwInfo" yaml:"hwInfo"`                         // Hardware information from HAL
	USBConnectionsInfo string `json:"usbConnectionsInfo" yaml:"usbConnectionsInfo"` // USB connections information from HAL
}

// NewResourceManagerStatus creates a new ResourceManagerStatus
func NewResourceManagerStatus() *ResourceManagerStatus {
	return &ResourceManagerStatus{}
}

// SetHWInfo sets the hardware info and returns the status for chaining
func (r *ResourceManagerStatus) SetHWInfo(hwInfo string) *ResourceManagerStatus {
	r.HWInfo = hwInfo
	return r
}

// SetUSBConnectionsInfo sets the USB connections info and returns the status for chaining
func (r *ResourceManagerStatus) SetUSBConnectionsInfo(usbInfo string) *ResourceManagerStatus {
	r.USBConnectionsInfo = usbInfo
	return r
}
