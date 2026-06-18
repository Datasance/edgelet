package docker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
)

// ContainerStats represents container statistics
type ContainerStats struct {
	CPUUsage    float32
	MemoryUsage int64
	Timestamp   time.Time
}

// GetContainerStats gets container statistics
func (c *Client) GetContainerStats(containerID string) (*ContainerStats, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, errors.New("docker client not initialized")
	}

	ctx, cancel := context.WithTimeout(c.GetContext(), 2*time.Second)
	defer cancel()

	// Get stats stream
	statsResult, err := cli.ContainerStats(ctx, containerID, client.ContainerStatsOptions{})
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = statsResult.Body.Close()
	}()

	// Read first stats response
	var stats container.StatsResponse
	if err := json.NewDecoder(statsResult.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	// Calculate CPU usage percentage
	cpuUsage := calculateCPUUsage(&stats)

	// Get memory usage
	memoryUsage := int64(stats.MemoryStats.Usage) // #nosec G115 -- container memory usage is below int64 max in practice

	return &ContainerStats{
		CPUUsage:    cpuUsage,
		MemoryUsage: memoryUsage,
		Timestamp:   time.Now(),
	}, nil
}

// calculateCPUUsage calculates CPU usage percentage from Docker stats
func calculateCPUUsage(stats *container.StatsResponse) float32 {
	if stats.CPUStats.CPUUsage.TotalUsage == 0 {
		return 0.0
	}

	// Calculate CPU percentage
	// Formula: (cpuDelta / systemDelta) * 100.0
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage)

	if systemDelta == 0 {
		return 0.0
	}

	// Get number of CPUs
	numCPUs := len(stats.CPUStats.CPUUsage.PercpuUsage)
	if numCPUs == 0 {
		numCPUs = 1
	}

	cpuPercent := (cpuDelta / systemDelta) * float64(numCPUs) * 100.0
	return float32(cpuPercent)
}
