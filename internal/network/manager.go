package network

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	moduleName = "Network Interface Manager"
)

// NetworkInterfaceInfo contains information about a network interface
// NetworkInterfaceInfo describes a network interface on the host.
//
//nolint:revive // exported API
type NetworkInterfaceInfo struct {
	Interface *net.Interface
	Address   net.IP
}

// Manager manages network interface detection and IP address management
type Manager struct {
	currentIPAddress string
	networkInterface *NetworkInterfaceInfo
	hostName         string
	pid              int
	mu               sync.RWMutex
	config           *config.Config
	ctx              context.Context
	cancel           context.CancelFunc
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton Network Interface Manager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config: config.GetInstance(),
		}
		instance.ctx, instance.cancel = context.WithCancel(context.Background())
	})
	return instance
}

// Start starts the Network Interface Manager
func (m *Manager) Start() error {
	logging.LogInfo(moduleName, "Start IoFog NetworkInterface")

	// Initial update
	if err := m.UpdateNetworkInterface(); err != nil {
		logging.LogError(moduleName, "Error in updating IOFogNetworkInterface", err)
		// Retry on error
		return m.Start()
	}

	// Start periodic updates (every 30 minutes)
	go m.periodicUpdate()

	logging.LogInfo(moduleName, "Started IoFog NetworkInterface")
	return nil
}

// Stop stops the Network Interface Manager
func (m *Manager) Stop() error {
	logging.LogInfo(moduleName, "Stopping Network Interface Manager")
	if m.cancel != nil {
		m.cancel()
	}
	return nil
}

// periodicUpdate periodically updates network interface information (every 30 minutes)
func (m *Manager) periodicUpdate() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.UpdateNetworkInterface(); err != nil {
				logging.LogError(moduleName, "Error updating network interface", err)
				// Restart the periodic update on error
				go m.periodicUpdate()
				return
			}
		}
	}
}

// UpdateNetworkInterface updates the network interface information
func (m *Manager) UpdateNetworkInterface() error {
	logging.LogDebug(moduleName, "Updating network interface")

	m.mu.Lock()
	defer m.mu.Unlock()

	// Get network interface first (this will also help us get IP)
	netInterface, err := m.getNetworkInterface()
	if err != nil {
		// Log warning but don't fail - try to get IP from any interface
		logging.LogWarn(moduleName, fmt.Sprintf("Unable to get network interface: %v, trying fallback", err))
		// Try fallback: get any non-loopback IPv4 address
		ipAddress, fallbackErr := m.getAnyIPv4Address()
		if fallbackErr != nil {
			return fmt.Errorf("unable to get network interface or fallback IP: %w (fallback: %w)", err, fallbackErr)
		}
		m.currentIPAddress = ipAddress
		m.networkInterface = nil // Set to nil if we couldn't find the specific interface
	} else {
		m.networkInterface = netInterface
		// Get IP from the found interface
		if netInterface != nil && netInterface.Address != nil {
			m.currentIPAddress = netInterface.Address.String()
		} else {
			// Fallback to any IPv4 address
			ipAddress, fallbackErr := m.getAnyIPv4Address()
			if fallbackErr != nil {
				return fmt.Errorf("unable to get IP address from interface: %w", fallbackErr)
			}
			m.currentIPAddress = ipAddress
		}
	}

	// Store IP address in memory only
	// Note: IP address is NOT saved to config file - it's only stored in memory
	if m.currentIPAddress != "" {
		logging.LogDebug(moduleName, fmt.Sprintf("Updated network interface IP address in memory: %s", m.currentIPAddress))
	} else {
		logging.LogWarn(moduleName, "No IP address detected")
	}

	// Get hostname
	hostname, err := os.Hostname()
	if err != nil {
		logging.LogWarn(moduleName, "Unable to get hostname, setting as unknown-host")
		hostname = "unknown-host"
	}
	m.hostName = hostname

	// Get PID
	m.pid = os.Getpid()

	configuredInterface := strings.TrimSpace(m.config.NetworkInterface)
	if configuredInterface == "" {
		configuredInterface = "dynamic"
	}
	selectedInterface := "unresolved"
	if m.networkInterface != nil && m.networkInterface.Interface != nil {
		if name := strings.TrimSpace(m.networkInterface.Interface.Name); name != "" {
			selectedInterface = name
		}
	}
	if strings.EqualFold(configuredInterface, "dynamic") {
		logging.LogInfo(moduleName, fmt.Sprintf("Network interface selection: mode=dynamic selected=%s ip=%s", selectedInterface, m.currentIPAddress))
	} else {
		logging.LogInfo(moduleName, fmt.Sprintf("Network interface selection: mode=static configured=%s selected=%s ip=%s", configuredInterface, selectedInterface, m.currentIPAddress))
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Network interface updated: IP=%s, Hostname=%s, PID=%d", m.currentIPAddress, m.hostName, m.pid))
	return nil
}

// getAnyIPv4Address gets any non-loopback IPv4 address as fallback
func (m *Manager) getAnyIPv4Address() (string, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", fmt.Errorf("failed to get network interfaces: %w", err)
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Scanning %d network interfaces", len(interfaces)))

	for _, iface := range interfaces {
		// Skip loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			logging.LogDebug(moduleName, fmt.Sprintf("Skipping loopback interface: %s", iface.Name))
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			logging.LogDebug(moduleName, fmt.Sprintf("Skipping down interface: %s", iface.Name))
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			logging.LogDebug(moduleName, fmt.Sprintf("Error getting addresses for interface %s: %v", iface.Name, err))
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip != nil && !ip.IsLoopback() && ip.To4() != nil {
				logging.LogInfo(moduleName, fmt.Sprintf("Found IPv4 address %s on interface %s", ip.String(), iface.Name))
				return ip.String(), nil
			}
		}
	}

	logging.LogWarn(moduleName, "No suitable IPv4 address found on any interface")
	return "", errors.New("no suitable IPv4 address found")
}

// getNetworkInterface gets the network interface based on controller URL
func (m *Manager) getNetworkInterface() (*NetworkInterfaceInfo, error) {
	return m.resolveNetworkInterface(m.config.ControllerURL, m.config.NetworkInterface)
}

// ValidateNetworkInterfaceConfig validates whether a config update can use the requested
// network interface without mutating manager state.
func (m *Manager) ValidateNetworkInterfaceConfig(controllerURL, networkInterfaceConfig string) error {
	normalizedInterface := strings.TrimSpace(networkInterfaceConfig)
	if normalizedInterface == "" || strings.EqualFold(normalizedInterface, "dynamic") {
		return nil
	}
	resolved, err := m.resolveNetworkInterface(controllerURL, normalizedInterface)
	if err != nil {
		return err
	}
	if resolved == nil || resolved.Interface == nil {
		return fmt.Errorf("no usable address found on network interface %s", normalizedInterface)
	}
	return nil
}

// GetAvailableNetworkInterfaces returns available non-loopback interface names.
func (m *Manager) GetAvailableNetworkInterfaces() []string {
	interfaces, err := net.Interfaces()
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Unable to list network interfaces: %v", err))
		return nil
	}
	names := make([]string, 0, len(interfaces))
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		name := strings.TrimSpace(iface.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func (m *Manager) resolveNetworkInterface(controllerURL, networkInterfaceConfig string) (*NetworkInterfaceInfo, error) {
	if strings.TrimSpace(controllerURL) == "" {
		return nil, errors.New("controller URL not configured")
	}
	normalizedInterface := strings.TrimSpace(networkInterfaceConfig)
	if normalizedInterface != "" && !strings.EqualFold(normalizedInterface, "dynamic") {
		// Use specific network interface
		return m.getSpecificNetworkInterface(normalizedInterface, controllerURL)
	}
	// Use dynamic detection
	return m.getOSNetworkInterface(controllerURL)
}

// getSpecificNetworkInterface gets a specific network interface by name
func (m *Manager) getSpecificNetworkInterface(interfaceName, controllerURL string) (*NetworkInterfaceInfo, error) {
	// Parse controller URL
	parsedURL, err := url.Parse(controllerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse controller URL: %w", err)
	}

	controllerHost := parsedURL.Hostname()
	if controllerHost == "" {
		controllerHost, err = extractHostFromURL(controllerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to extract host from URL: %w", err)
		}
	}

	controllerPort := parsedURL.Port()
	if controllerPort == "" {
		if parsedURL.Scheme == "https" {
			controllerPort = "443"
		} else {
			controllerPort = "80"
		}
	}

	// Get network interface by name
	iface, err := net.InterfaceByName(interfaceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get network interface %s: %w", interfaceName, err)
	}

	// Check if this interface can connect to controller
	return m.getConnectedAddress(parsedURL, controllerHost, controllerPort, iface, true), nil
}

// getOSNetworkInterface gets the OS network interface (dynamic detection)
func (m *Manager) getOSNetworkInterface(controllerURL string) (*NetworkInterfaceInfo, error) {
	// Parse controller URL properly
	parsedURL, err := url.Parse(controllerURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse controller URL: %w", err)
	}

	controllerHost := parsedURL.Hostname()
	if controllerHost == "" {
		// Fallback to old extraction method
		controllerHost, err = extractHostFromURL(controllerURL)
		if err != nil {
			return nil, fmt.Errorf("failed to extract host from URL: %w", err)
		}
	}

	// Get controller port
	controllerPort := parsedURL.Port()
	if controllerPort == "" {
		// Use default ports based on scheme
		if parsedURL.Scheme == "https" {
			controllerPort = "443"
		} else {
			controllerPort = "80"
		}
	}

	// Get CNI bridge interface name (same on full and lite).
	cniBridgeInterfaceName := m.getCNIBridgeInterfaceName()

	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var cniBridgeInterface *net.Interface

	// First pass: find interfaces that can connect to controller.
	// Skip the container bridge on first pass
	for _, iface := range interfaces {
		if cniBridgeInterfaceName != "" && iface.Name == cniBridgeInterfaceName {
			cniBridgeInterface = &iface
			continue
		}

		// Skip loopback, virtual, and down interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Check if this interface can connect to controller
		connectedAddr := m.getConnectedAddress(parsedURL, controllerHost, controllerPort, &iface, true)
		if connectedAddr != nil {
			logging.LogInfo(moduleName, fmt.Sprintf("Detected network interface: %s with address: %s", iface.Name, connectedAddr.Address.String()))
			return connectedAddr, nil
		}
	}

	// If no interface found and the CNI bridge exists, try it without connection check.
	if cniBridgeInterface != nil {
		connectedAddr := m.getConnectedAddress(parsedURL, controllerHost, controllerPort, cniBridgeInterface, false)
		if connectedAddr != nil {
			logging.LogInfo(moduleName, fmt.Sprintf("Using CNI bridge interface: %s with address: %s", cniBridgeInterface.Name, connectedAddr.Address.String()))
			return connectedAddr, nil
		}
	}

	return nil, errors.New("no suitable network interface found")
}

// getConnectedAddress checks if a network interface can connect to the controller
func (m *Manager) getConnectedAddress(_ *url.URL, controllerHost, controllerPort string, networkInterface *net.Interface, checkConnection bool) *NetworkInterfaceInfo {
	addrs, err := networkInterface.Addrs()
	if err != nil {
		return nil
	}

	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		}

		if ip == nil || ip.IsLoopback() {
			continue
		}

		// Only check IPv4 addresses
		if ip.To4() == nil {
			continue
		}

		logging.LogInfo(moduleName, fmt.Sprintf("Detected network interface: %s And network address - hostAddress: %s type of: IPv4", networkInterface.Name, ip.String()))

		// If not checking connection, return first valid IPv4 address
		if !checkConnection {
			return &NetworkInterfaceInfo{
				Interface: networkInterface,
				Address:   ip,
			}
		}

		// Test connection to controller
		if m.testConnection(ip, controllerHost, controllerPort) {
			return &NetworkInterfaceInfo{
				Interface: networkInterface,
				Address:   ip,
			}
		}
	}

	return nil
}

// testConnection tests if an IP can connect to the controller
func (m *Manager) testConnection(localIP net.IP, controllerHost, controllerPort string) bool {
	// Create a dialer bound to the local IP
	dialer := &net.Dialer{
		LocalAddr: &net.TCPAddr{
			IP: localIP,
		},
		Timeout: 1 * time.Second,
	}

	// Try to connect to controller
	conn, err := dialer.Dial("tcp", net.JoinHostPort(controllerHost, controllerPort))
	if err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Unable to Get Connected Address: %v", err))
		return false
	}
	if err := conn.Close(); err != nil {
		logging.LogWarn(moduleName, fmt.Sprintf("Failed to close test connection: %v", err))
	}
	return true
}

// extractHostFromURL extracts the host from a URL (fallback method)
func extractHostFromURL(urlStr string) (string, error) {
	// Simple extraction - remove protocol prefix
	if len(urlStr) > 7 && urlStr[:7] == "http://" {
		urlStr = urlStr[7:]
	} else if len(urlStr) > 8 && urlStr[:8] == "https://" {
		urlStr = urlStr[8:]
	}

	// Remove path and port
	for i, char := range urlStr {
		if char == '/' {
			return urlStr[:i], nil
		}
		if char == ':' {
			// Check if this is a port separator (not IPv6)
			if i+1 < len(urlStr) && urlStr[i+1] != ':' {
				return urlStr[:i], nil
			}
		}
	}

	return urlStr, nil
}

// GetCurrentIPAddress returns the current IP address
func (m *Manager) GetCurrentIPAddress() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentIPAddress
}

// GetNetworkInterface returns the network interface information
func (m *Manager) GetNetworkInterface() *NetworkInterfaceInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.networkInterface
}

// GetHostName returns the hostname
func (m *Manager) GetHostName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.hostName
}

// GetPID returns the process ID
func (m *Manager) GetPID() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.pid
}

// GetName returns the module name
func (m *Manager) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index (Network Interface Manager doesn't have a specific index)
func (m *Manager) GetModuleIndex() int {
	return -1 // Network Interface Manager is not tracked in status
}
