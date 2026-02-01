package models

import "encoding/json"

// SshProxyManagerStatus represents the SSH Proxy Manager status
type SshProxyManagerStatus struct {
	ProxyStatus map[string]interface{} `json:"proxyStatus" yaml:"proxyStatus"` // Proxy status information
}

// NewSshProxyManagerStatus creates a new SshProxyManagerStatus
func NewSshProxyManagerStatus() *SshProxyManagerStatus {
	return &SshProxyManagerStatus{
		ProxyStatus: make(map[string]interface{}),
	}
}

// SetProxyStatus sets the proxy status and returns the status for chaining
func (s *SshProxyManagerStatus) SetProxyStatus(status map[string]interface{}) *SshProxyManagerStatus {
	s.ProxyStatus = status
	return s
}

// GetJSONProxyStatus returns the proxy status as a JSON string
func (s *SshProxyManagerStatus) GetJSONProxyStatus() string {
	jsonData, err := json.Marshal(s.ProxyStatus)
	if err != nil {
		return "{}"
	}
	return string(jsonData)
}

// SetProxyConfig sets the proxy configuration
func (s *SshProxyManagerStatus) SetProxyConfig(username, host string, remotePort, localPort int) *SshProxyManagerStatus {
	if s.ProxyStatus == nil {
		s.ProxyStatus = make(map[string]interface{})
	}
	s.ProxyStatus["username"] = username
	s.ProxyStatus["host"] = host
	s.ProxyStatus["remotePort"] = remotePort
	s.ProxyStatus["localPort"] = localPort
	return s
}

// SetConnectionStatus sets the connection status
func (s *SshProxyManagerStatus) SetConnectionStatus(status string) *SshProxyManagerStatus {
	if s.ProxyStatus == nil {
		s.ProxyStatus = make(map[string]interface{})
	}
	s.ProxyStatus["status"] = status
	return s
}

// SetErrorMessage sets the error message
func (s *SshProxyManagerStatus) SetErrorMessage(errMsg string) *SshProxyManagerStatus {
	if s.ProxyStatus == nil {
		s.ProxyStatus = make(map[string]interface{})
	}
	if errMsg != "" {
		s.ProxyStatus["errorMessage"] = errMsg
	} else {
		delete(s.ProxyStatus, "errorMessage")
	}
	return s
}
