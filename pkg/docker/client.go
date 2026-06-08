package docker

import (
	"context"
	"sync"

	"github.com/docker/docker/client"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

const (
	ModuleName = "Docker Client"
)

// Client wraps the Docker client with reconnection logic
type Client struct {
	client     *client.Client
	ctx        context.Context
	cancel     context.CancelFunc
	mu         sync.RWMutex
	dockerURL  string
	apiVersion string
	logger     *logging.ModuleLogger
}

var (
	instance *Client
	once     sync.Once
)

// GetInstance returns the singleton Docker client instance
func GetInstance() *Client {
	once.Do(func() {
		instance = &Client{
			logger: logging.NewModuleLogger(ModuleName),
		}
	})
	return instance
}

// Init initializes the Docker client with the given configuration
func (c *Client) Init(dockerURL, apiVersion string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.dockerURL = dockerURL
	c.apiVersion = apiVersion

	return c.initDockerClient()
}

// initDockerClient initializes the Docker client
func (c *Client) initDockerClient() error {
	c.logger.Info("Start Docker Client initialization")

	// Create context
	if c.cancel != nil {
		c.cancel()
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Build client options
	opts := []client.Opt{
		client.WithHost(c.dockerURL),
		client.WithAPIVersionNegotiation(),
	}

	// Set API version if specified
	if c.apiVersion != "" {
		opts = append(opts, client.WithVersion(c.apiVersion))
	}

	// Create Docker client
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		c.logger.Errorf("Docker client initialization failed: %v", err)
		return err
	}

	// Close old client if exists
	if c.client != nil {
		if err := c.client.Close(); err != nil {
			c.logger.Warnf("Failed to close old Docker client: %v", err)
		}
	}

	c.client = cli

	// Ensure "edgelet" bridge network exists before returning
	// call in initDockerClient(). Use the lock-free variant because c.mu is already
	// held by Init() / ReInit(); calling ensureIoFogNetworkExists() here would deadlock.
	if err := c.ensureNetworkLockFree(c.ctx, cli); err != nil {
		c.logger.Warnf("Failed to ensure iofog network exists: %v", err)
	}

	// Start Docker events handler
	go c.addDockerEventHandler()

	c.logger.Info("Finished Docker Client initialization")
	return nil
}

// ReInit reinitializes the Docker client
func (c *Client) ReInit() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.logger.Info("Start Docker Client re-initialization")

	if c.client != nil {
		if err := c.client.Close(); err != nil {
			c.logger.Warnf("Docker client closing failed: %v", err)
		}
	}

	return c.initDockerClient()
}

// GetClient returns the underlying Docker client (thread-safe)
func (c *Client) GetClient() *client.Client {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.client
}

// GetContext returns the context for Docker operations
func (c *Client) GetContext() context.Context {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ctx
}

// Close closes the Docker client
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cancel != nil {
		c.cancel()
	}

	if c.client != nil {
		return c.client.Close()
	}

	return nil
}
