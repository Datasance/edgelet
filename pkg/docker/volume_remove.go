package docker

import (
	"errors"
	"strings"
)

// RemoveNamedVolume deletes a docker named volume when it exists.
func (c *Client) RemoveNamedVolume(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("volume name is required")
	}
	cli := c.GetClient()
	if cli == nil {
		return errors.New("docker client is not initialized")
	}
	return cli.VolumeRemove(c.GetContext(), name, true)
}
