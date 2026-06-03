package docker

import (
	"fmt"
	"strings"
)

// RemoveNamedVolume deletes a docker named volume when it exists.
func (c *Client) RemoveNamedVolume(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("volume name is required")
	}
	cli := c.GetClient()
	if cli == nil {
		return fmt.Errorf("docker client is not initialized")
	}
	return cli.VolumeRemove(c.GetContext(), name, true)
}
