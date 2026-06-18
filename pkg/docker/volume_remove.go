package docker

import (
	"errors"
	"strings"

	"github.com/moby/moby/client"
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
	_, err := cli.VolumeRemove(c.GetContext(), name, client.VolumeRemoveOptions{Force: true})
	return err
}
