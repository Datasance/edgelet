package docker

import (
	"context"
	"errors"

	"github.com/docker/docker/api/types"
	"github.com/eclipse-iofog/agent/internal/models"
)

// addDockerEventHandler starts the Docker events handler
func (c *Client) addDockerEventHandler() {
	c.logger.Debug("Starting docker events handler")

	cli := c.GetClient()
	if cli == nil {
		c.logger.Error("Docker client not initialized")
		return
	}

	ctx := c.GetContext()

	// Start listening to Docker events
	events, errs := cli.Events(ctx, types.EventsOptions{})

	go func() {
		for {
			select {
			case event := <-events:
				if event.Type == "container" || event.Type == "image" {
					// Docker events are logged here for debugging
					// Status updates are handled by ProcessManager which monitors containers
					// and updates status reporter accordingly
					state := models.MicroserviceStateFromText(event.Status)
					c.logger.Debugf("Docker event: Type=%s, Status=%s, ID=%s, State=%s", event.Type, event.Status, event.ID, state)
					_ = state // Status updates handled by ProcessManager
				}
			case err := <-errs:
				if err != nil {
					c.logger.Errorf("Docker events error: %v", err)
					// Try to reconnect
					if errors.Is(err, context.Canceled) {
						return
					}
					// Reinitialize client on error
					if err := c.ReInit(); err != nil {
						c.logger.Errorf("Failed to reinitialize Docker client: %v", err)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	c.logger.Debug("Docker events handler started")
}
