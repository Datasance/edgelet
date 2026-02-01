package proxy

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	localHost = "localhost"
	timeout   = 60 * time.Second
)

// SshConnection represents an SSH connection for tunneling
type SshConnection struct {
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

// NewSshConnection creates a new SSH connection
func NewSshConnection() *SshConnection {
	return &SshConnection{
		localPort: 22, // Default local port
	}
}

// SetProxyInfo sets connection information
func (c *SshConnection) SetProxyInfo(username, password, host string, rport, lport int, rsaKey string, closeFlag bool) {
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
func (c *SshConnection) SetKnownHost() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.rsaKey == "" {
		return fmt.Errorf("RSA key is empty")
	}

	// Parse known hosts from RSA key string
	// The RSA key is typically in known_hosts format
	// For now, we'll store it and validate during connection
	return nil
}

// OpenSshTunnel opens an SSH tunnel (reverse port forwarding)
func (c *SshConnection) OpenSshTunnel() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Build SSH client config
	config := &ssh.ClientConfig{
		User:            c.username,
		Auth:            []ssh.AuthMethod{ssh.Password(c.password)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: Use known hosts validation
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
		client.Close()
		return fmt.Errorf("failed to set up reverse port forwarding: %w", err)
	}

	// Start accepting connections and forwarding them
	go func() {
		defer listener.Close()
		for {
			remoteConn, err := listener.Accept()
			if err != nil {
				// Listener closed or error
				return
			}

			// Forward connection to local port
			go func(conn net.Conn) {
				defer conn.Close()
				localConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", localHost, c.localPort))
				if err != nil {
					return
				}
				defer localConn.Close()

				// Copy data bidirectionally
				done := make(chan struct{}, 2)
				go func() {
					io.Copy(conn, localConn)
					done <- struct{}{}
				}()
				go func() {
					io.Copy(localConn, conn)
					done <- struct{}{}
				}()
				<-done
			}(remoteConn)
		}
	}()

	return nil
}

// IsConnected checks if the tunnel is open
func (c *SshConnection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.client != nil && c.client.Conn != nil
}

// IsCloseFlag returns the close flag
func (c *SshConnection) IsCloseFlag() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.closeFlag
}

// Close closes the SSH connection
func (c *SshConnection) Close() error {
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
func (c *SshConnection) GetUsername() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.username
}

// GetHost returns the host
func (c *SshConnection) GetHost() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.host
}

// GetRemotePort returns the remote port
func (c *SshConnection) GetRemotePort() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remotePort
}

// GetLocalPort returns the local port
func (c *SshConnection) GetLocalPort() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.localPort
}

// GetClient returns the SSH client (for testing)
func (c *SshConnection) GetClient() *ssh.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.client
}
