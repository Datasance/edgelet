package models

import "encoding/json"

// SSHProxyManagerStatus represents the SSH Proxy Manager status
type SSHProxyManagerStatus struct {
	ProxyStatus map[string]any `json:"proxyStatus" yaml:"proxyStatus"` // Proxy status information
}

// NewSSHProxyManagerStatus creates a new SSHProxyManagerStatus
func NewSSHProxyManagerStatus() *SSHProxyManagerStatus {
	return &SSHProxyManagerStatus{
		ProxyStatus: make(map[string]any),
	}
}

// SetProxyStatus sets the proxy status and returns the status for chaining
func (s *SSHProxyManagerStatus) SetProxyStatus(status map[string]any) *SSHProxyManagerStatus {
	s.ProxyStatus = status
	return s
}

// GetJSONProxyStatus returns the proxy status as a JSON string
func (s *SSHProxyManagerStatus) GetJSONProxyStatus() string {
	jsonData, err := json.Marshal(s.ProxyStatus)
	if err != nil {
		return "{}"
	}
	return string(jsonData)
}

// SetProxyConfig sets the proxy configuration
func (s *SSHProxyManagerStatus) SetProxyConfig(username, host string, remotePort, localPort int) *SSHProxyManagerStatus {
	if s.ProxyStatus == nil {
		s.ProxyStatus = make(map[string]any)
	}
	s.ProxyStatus["username"] = username
	s.ProxyStatus["host"] = host
	s.ProxyStatus["remotePort"] = remotePort
	s.ProxyStatus["localPort"] = localPort
	return s
}

// SetConnectionStatus sets the connection status
func (s *SSHProxyManagerStatus) SetConnectionStatus(status string) *SSHProxyManagerStatus {
	if s.ProxyStatus == nil {
		s.ProxyStatus = make(map[string]any)
	}
	s.ProxyStatus["status"] = status
	return s
}

// SetErrorMessage sets the error message
func (s *SSHProxyManagerStatus) SetErrorMessage(errMsg string) *SSHProxyManagerStatus {
	if s.ProxyStatus == nil {
		s.ProxyStatus = make(map[string]any)
	}
	if errMsg != "" {
		s.ProxyStatus["errorMessage"] = errMsg
	} else {
		delete(s.ProxyStatus, "errorMessage")
	}
	return s
}
