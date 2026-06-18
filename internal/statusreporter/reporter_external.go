//go:build !linux || !cgo

package statusreporter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/moby/moby/client"
)

var listExternalRuntimesForStatus = func(_ string) ([]string, error) {
	cfg := config.GetInstance()
	if cfg == nil {
		return nil, errors.New("config is not initialized")
	}

	engineURL := strings.TrimSpace(cfg.ContainerEngineURL)
	apiVersion := strings.TrimSpace(cfg.DockerAPIVersion)
	var cli *client.Client
	var err error
	if apiVersion != "" {
		cli, err = client.New(
			client.WithHost(engineURL),
			client.WithAPIVersion(apiVersion),
		)
	} else {
		cli, err = client.New(
			client.WithHost(engineURL),
		)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = cli.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	infoResult, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, err
	}
	info := infoResult.Info
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
