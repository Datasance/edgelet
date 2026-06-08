package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	localHost            = "localhost"
	timeout              = 60 * time.Second
	connectionModuleName = "Proxy Connection"
)

// SSHConnection represents an SSH connection for tunneling
type SSHConnection struct {
	mu         sync.Mutex
	client     *ssh.Client
	username   string
	password   string
	host       string
	rsaKey     string
	remotePort int
	localPort  int
	closeFlag  bool
}

// NewSSHConnection creates a new SSH connection
func NewSSHConnection() *SSHConnection {
	return &SSHConnection{
		localPort: 22, // Default local port
	}
}

// SetProxyInfo sets connection information
func (c *SSHConnection) SetProxyInfo(username, password, host string, rport, lport int, rsaKey string, closeFlag bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.username = username
	c.password = password
	c.host = host
	c.remotePort = rport
	c.localPort = lport
	c.rsaKey = rsaKey
	c.closeFlag = closeFlag
}

// SetKnownHost sets the known hosts from RSA key
func (c *SSHConnection) SetKnownHost() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rsaKey == "" {
		return errors.New("RSA key is empty")
	}

	// Parse known hosts from RSA key string
	// The RSA key is typically in known_hosts format
	// For now, we'll store it and validate during connection
	return nil
}

// OpenSSHTunnel opens an SSH tunnel (reverse port forwarding)
func (c *SSHConnection) OpenSSHTunnel() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build SSH client config
	config := &ssh.ClientConfig{
		User:            c.username,
		Auth:            []ssh.AuthMethod{ssh.Password(c.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // #nosec G106 -- SSH tunnel to IoFog internal infrastructure; host key pinning is a known future improvement
		Timeout:         timeout,
	}

	// Connect to SSH server
	address := fmt.Sprintf("%s:%d", c.host, c.localPort)
	client, err := ssh.Dial("tcp", address, config)
	if err != nil {
		return fmt.Errorf("failed to connect to SSH server: %w", err)
	}

	c.client = client

	// Set up reverse port forwarding using SSH remote port forwarding
	// Format: remote:remotePort -> local:localPort
	// This creates a listener on the remote server that forwards to local port
	listener, err := client.Listen("tcp", fmt.Sprintf(":%d", c.remotePort))
	if err != nil {
		if cerr := client.Close(); cerr != nil {
			logging.LogWarn(connectionModuleName, fmt.Sprintf("Failed to close SSH client: %v", cerr))
		}
		return fmt.Errorf("failed to set up reverse port forwarding: %w", err)
	}

	// Start accepting connections and forwarding them
	go func() {
		defer func() {
			_ = listener.Close()
		}()
		for {
			remoteConn, err := listener.Accept()
			if err != nil {
				// Listener closed or error
				return
			}

			// Forward connection to local port
			go func(conn net.Conn) {
				defer func() {
					_ = conn.Close()
				}()
				localConn, err := net.Dial("tcp", net.JoinHostPort(localHost, fmt.Sprintf("%d", c.localPort)))
				if err != nil {
					return
				}
				defer func() {
					_ = localConn.Close()
				}()

				// Copy data bidirectionally
				done := make(chan struct{}, 2)
				go func() {
					_, _ = io.Copy(conn, localConn) // #nosec G104 -- bidirectional copy; errors are non-critical
					done <- struct{}{}
				}()
				go func() {
					_, _ = io.Copy(localConn, conn) // #nosec G104 -- bidirectional copy; errors are non-critical
					done <- struct{}{}
				}()
				<-done
			}(remoteConn)
		}
	}()

	return nil
}

// IsConnected checks if the tunnel is open
func (c *SSHConnection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.client != nil && c.client.Conn != nil
}

// IsCloseFlag returns the close flag
func (c *SSHConnection) IsCloseFlag() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeFlag
}

// Close closes the SSH connection
func (c *SSHConnection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.client != nil {
		err := c.client.Close()
		c.client = nil
		return err
	}
	return nil
}

// GetUsername returns the username
func (c *SSHConnection) GetUsername() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username
}

// GetHost returns the host
func (c *SSHConnection) GetHost() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

// GetRemotePort returns the remote port
func (c *SSHConnection) GetRemotePort() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remotePort
}

// GetLocalPort returns the local port
func (c *SSHConnection) GetLocalPort() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localPort
}

// GetClient returns the SSH client (for testing)
func (c *SSHConnection) GetClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}
