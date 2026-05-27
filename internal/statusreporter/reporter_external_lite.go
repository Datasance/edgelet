//go:build lite

package statusreporter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/docker/docker/client"
)

var listExternalRuntimesForStatus = func(_ string) ([]string, error) {
	cfg := config.GetInstance()
	if cfg == nil {
		return nil, errors.New("config is not initialized")
	}

	opts := []client.Opt{
		client.WithHost(strings.TrimSpace(cfg.DockerURL)),
		client.WithAPIVersionNegotiation(),
	}
	if strings.TrimSpace(cfg.DockerAPIVersion) != "" {
		opts = append(opts, client.WithVersion(strings.TrimSpace(cfg.DockerAPIVersion)))
	}
	cli, err := client.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	info, err := cli.Info(ctx)
	if err != nil {
		return nil, err
	}
	runtimes := make([]string, 0, len(info.Runtimes))
	for runtimeName := range info.Runtimes {
		runtimeName = strings.ToLower(strings.TrimSpace(runtimeName))
		if runtimeName == "" {
			continue
		}
		runtimes = append(runtimes, runtimeName)
	}
	return sortedUniqueStrings(runtimes), nil
}

var listCatalogRuntimesForStatus = func() []string {
	return nil
}
