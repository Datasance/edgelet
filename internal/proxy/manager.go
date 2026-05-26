package proxy

import (
	"fmt"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/models"
	"github.com/datasance/edgelet/internal/statusreporter"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	moduleName        = "SSH Proxy Manager"
	defaultLocalPort  = 22
	defaultRemotePort = 9999
)

// SSHConnectionStatus represents SSH connection status
type SSHConnectionStatus string

const (
	SSHConnectionStatusOpen   SSHConnectionStatus = "OPEN"
	SSHConnectionStatusClosed SSHConnectionStatus = "CLOSED"
	SSHConnectionStatusFailed SSHConnectionStatus = "FAILED"
)

// Manager manages SSH proxy connections
type Manager struct {
	connection  *SSHConnection
	mu          sync.Mutex
	monitoring  bool
	stopMonitor chan struct{}
	logger      *logging.ModuleLogger
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton SSH Proxy Manager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			connection:  NewSSHConnection(),
			stopMonitor: make(chan struct{}),
			logger:      logging.NewModuleLogger(moduleName),
		}
	})
	return instance
}

// Update starts or stops SSH tunnel according to current config
func (m *Manager) Update(config map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(config) == 0 {
		m.handleUnexpectedTunnelState("Received invalid proxy config", SSHConnectionStatusFailed)
		return fmt.Errorf("invalid proxy config")
	}

	m.setSSHConnection(config)
	return m.processValidConfig()
}

// processValidConfig processes valid configuration
func (m *Manager) processValidConfig() error {
	if m.connection.IsConnected() {
		if m.connection.IsCloseFlag() {
			m.close()
		} else {
			m.handleUnexpectedTunnelState("The tunnel is already opened. Please close it first.", SSHConnectionStatusOpen)
		}
		return nil
	}

	if m.connection.IsCloseFlag() {
		m.handleUnexpectedTunnelState("The tunnel is already closed", SSHConnectionStatusClosed)
		return nil
	}

	return m.open()
}

// open opens SSH tunnel
func (m *Manager) open() error {
	// Set known host
	if err := m.connection.SetKnownHost(); err != nil {
		errMsg := fmt.Sprintf("There was an issue with server RSA key setup: %v", err)
		m.logger.Errorf("%s", errMsg)
		// Continue anyway
	}

	// Open SSH tunnel asynchronously
	go func() {
		if err := m.connection.OpenSSHTunnel(); err != nil {
			m.onError(err)
		} else {
			m.onSuccess()
		}
	}()

	return nil
}

// onSuccess handles successful tunnel opening
func (m *Manager) onSuccess() {
	m.setSSHProxyManagerStatus(SSHConnectionStatusOpen, "")
	m.logger.Info("opened ssh tunnel")

	// Start monitoring
	m.startMonitoring()
}

// onError handles tunnel opening errors
func (m *Manager) onError(err error) {
	errMsg := fmt.Sprintf("Unable to connect to the server: %v", err)
	m.logger.Errorf("%s", errMsg)
	m.setSSHProxyManagerStatus(SSHConnectionStatusFailed, errMsg)
}

// close closes SSH tunnel
func (m *Manager) close() {
	if err := m.connection.Close(); err != nil {
		m.logger.Warnf("Error closing SSH connection: %v", err)
	}
	m.stopMonitoring()
	m.setSSHProxyManagerStatus(SSHConnectionStatusClosed, "")
	m.logger.Info("closed ssh tunnel")
}

// handleUnexpectedTunnelState handles unexpected tunnel states
func (m *Manager) handleUnexpectedTunnelState(errMsg string, status SSHConnectionStatus) {
	m.logger.Warn(errMsg)
	if m.connection != nil && m.connection.GetUsername() != "" {
		m.setSSHProxyManagerStatus(status, errMsg)
	}
}

// startMonitoring starts monitoring the SSH tunnel
func (m *Manager) startMonitoring() {
	m.mu.Lock()
	if m.monitoring {
		m.mu.Unlock()
		return
	}
	m.monitoring = true
	m.mu.Unlock()

	go func() {
		cfg := config.GetInstance()
		ticker := time.NewTicker(time.Duration(cfg.MonitorSSHTunnelStatusFreqSeconds) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if m.connection.IsConnected() {
					m.logger.Info("ssh tunnel heartbeat message")
				} else {
					if !m.connection.IsCloseFlag() {
						m.setSSHProxyManagerStatus(SSHConnectionStatusClosed, "")
					}
					m.mu.Lock()
					m.monitoring = false
					m.mu.Unlock()
					return
				}
			case <-m.stopMonitor:
				m.mu.Lock()
				m.monitoring = false
				m.mu.Unlock()
				return
			}
		}
	}()
}

// stopMonitoring stops monitoring the SSH tunnel
func (m *Manager) stopMonitoring() {
	select {
	case m.stopMonitor <- struct{}{}:
	default:
	}
}

// setSSHProxyManagerStatus sets the SSH proxy manager status
func (m *Manager) setSSHProxyManagerStatus(status SSHConnectionStatus, errMsg string) {
	statusReporter := statusreporter.GetInstance()
	statusReporter.UpdateSSHProxyManagerStatus(func(s *models.SSHProxyManagerStatus) {
		s.SetProxyConfig(
			m.connection.GetUsername(),
			m.connection.GetHost(),
			m.connection.GetRemotePort(),
			m.connection.GetLocalPort(),
		).
			SetConnectionStatus(string(status)).
			SetErrorMessage(errMsg)
	})
}

// setSSHConnection sets proxy connection info from config
func (m *Manager) setSSHConnection(config map[string]interface{}) {
	username, _ := config["username"].(string)
	password, _ := config["password"].(string)
	host, _ := config["host"].(string)
	rsaKey, _ := config["rsakey"].(string)

	rport := defaultRemotePort
	if rportVal, ok := config["rport"].(float64); ok {
		rport = int(rportVal)
	}

	lport := defaultLocalPort
	if lportVal, ok := config["lport"].(float64); ok {
		lport = int(lportVal)
	}

	closeFlag := false
	if closeFlagVal, ok := config["closed"].(bool); ok {
		closeFlag = closeFlagVal
	}

	m.connection.SetProxyInfo(username, password, host, rport, lport, rsaKey, closeFlag)
}
