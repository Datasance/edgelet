package docker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
)

// ContainerStats represents container statistics
type ContainerStats struct {
	CPUUsage    float32
	MemoryUsage int64
	Timestamp   time.Time
}

// GetContainerStats gets container statistics
// This matches Java: getContainerStats() with callback pattern
func (c *Client) GetContainerStats(containerID string) (*ContainerStats, error) {
	cli := c.GetClient()
	if cli == nil {
		return nil, fmt.Errorf("Docker client not initialized")
	}

	ctx, cancel := context.WithTimeout(c.GetContext(), 2*time.Second)
	defer cancel()

	// Get stats stream
	statsStream, err := cli.ContainerStats(ctx, containerID, false)
	if err != nil {
		return nil, err
	}
	defer statsStream.Body.Close()

	// Read first stats response
	var stats types.StatsJSON
	if err := json.NewDecoder(statsStream.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("failed to decode stats: %w", err)
	}

	// Calculate CPU usage percentage
	cpuUsage := calculateCPUUsage(&stats)

	// Get memory usage
	memoryUsage := int64(stats.MemoryStats.Usage)

	return &ContainerStats{
		CPUUsage:    cpuUsage,
		MemoryUsage: memoryUsage,
		Timestamp:   time.Now(),
	}, nil
}

// calculateCPUUsage calculates CPU usage percentage from Docker stats
func calculateCPUUsage(stats *types.StatsJSON) float32 {
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
