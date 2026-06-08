package resourceconsumption

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/edgelet/internal/config"
	"github.com/eclipse-iofog/edgelet/internal/models"
	"github.com/eclipse-iofog/edgelet/internal/statusreporter"
	"github.com/eclipse-iofog/edgelet/internal/utils"
	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/process"
)

const (
	moduleName = "Resource Consumption Manager"
)

// Manager handles resource consumption monitoring
type Manager struct {
	config         *config.Config
	statusReporter *statusreporter.StatusReporter
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
	mu             sync.RWMutex

	// Limits (in bytes for memory/disk, percentage for CPU)
	diskLimit   int64
	cpuLimit    float64
	memoryLimit int64
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton ResourceConsumptionManager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config: config.GetInstance(),
		}
	})
	return instance
}

// Start starts the ResourceConsumptionManager
func (rcm *Manager) Start() error {
	logging.LogInfo(moduleName, "Starting Resource Consumption Manager")

	rcm.statusReporter = statusreporter.GetInstance()
	rcm.InstanceConfigUpdated()
	logging.LogDebug(moduleName, fmt.Sprintf("Resource limits set: Disk=%.2f GiB, Memory=%.2f MiB, CPU=%.2f%%",
		float64(rcm.diskLimit)/1_000_000_000, float64(rcm.memoryLimit)/1_000_000, rcm.cpuLimit))

	// Create context for cancellation
	rcm.ctx, rcm.cancel = context.WithCancel(context.Background())

	// Collect initial usage data immediately (before starting periodic worker)
	// This ensures status is available right away
	logging.LogDebug(moduleName, "Collecting initial resource usage data")
	rcm.collectUsageData()
	logging.LogDebug(moduleName, fmt.Sprintf("Initial resource usage: Memory=%.2f MiB, CPU=%.2f%%, Disk=%.2f GiB",
		rcm.statusReporter.GetResourceConsumptionManagerStatus().MemoryUsage,
		rcm.statusReporter.GetResourceConsumptionManagerStatus().CPUUsage,
		rcm.statusReporter.GetResourceConsumptionManagerStatus().DiskUsage))

	// Start background worker
	rcm.wg.Add(1)
	go rcm.runUsageDataWorker()

	logging.LogInfo(moduleName, "Resource Consumption Manager started")
	return nil
}

// collectUsageData collects resource usage data and updates status
// Extracted from getUsageDataWorker for immediate collection on startup
func (rcm *Manager) collectUsageData() {
	logging.LogDebug(moduleName, "Get usage data")

	memoryUsage := rcm.getMemoryUsage()
	cpuUsage := rcm.getCPUUsage()

	// Calculate disk usage (directories may not exist yet, which is OK)
	archivePath := filepath.Join(rcm.config.DiskDirectory, "messages", "archive")
	volumesPath := filepath.Join(rcm.config.DiskDirectory, "volumes")
	archiveDiskUsage := rcm.directorySize(archivePath)
	volumesDiskUsage := rcm.directorySize(volumesPath)
	diskUsage := archiveDiskUsage + volumesDiskUsage

	logging.LogDebug(moduleName, fmt.Sprintf("Disk usage: archive=%d bytes, volumes=%d bytes, total=%d bytes",
		archiveDiskUsage, volumesDiskUsage, diskUsage))

	availableMemory := rcm.getSystemAvailableMemory()
	totalCPU := rcm.getTotalCPU()
	availableDisk := rcm.getAvailableDisk()
	totalDiskSpace := rcm.getTotalDiskSpace()

	logging.LogDebug(moduleName, fmt.Sprintf("System resources: availableMemory=%d bytes, availableDisk=%d bytes, totalDiskSpace=%d bytes, totalCpu=%.2f%%",
		availableMemory, availableDisk, totalDiskSpace, totalCPU))

	// Update status atomically (fixes race condition)
	rcm.statusReporter.UpdateResourceConsumptionManagerStatus(func(status *models.ResourceConsumptionManagerStatus) {
		status.MemoryUsage = float64(memoryUsage) / 1_000_000 // bytes to MiB
		status.CPUUsage = cpuUsage
		status.DiskUsage = float64(diskUsage) / 1_000_000_000 // bytes to GiB
		status.MemoryViolation = memoryUsage > rcm.memoryLimit
		status.DiskViolation = diskUsage > rcm.diskLimit
		status.CPUViolation = cpuUsage > rcm.cpuLimit
		status.AvailableMemory = availableMemory
		status.AvailableDisk = availableDisk
		status.TotalDiskSpace = totalDiskSpace
		status.TotalCPU = totalCPU
	})

	logging.LogDebug(moduleName, fmt.Sprintf("Updated status: MemoryUsage=%.2f MiB, CPUUsage=%.2f%%, DiskUsage=%.2f GiB",
		float64(memoryUsage)/1_000_000, cpuUsage, float64(diskUsage)/1_000_000_000))

	// Prune archives if disk usage exceeds limit
	if diskUsage > rcm.diskLimit {
		amount := diskUsage - int64(float64(rcm.diskLimit)*0.75)
		rcm.removeArchives(amount)
	}

	logging.LogDebug(moduleName, "Finished Get usage data")
}

// Stop stops the ResourceConsumptionManager
func (rcm *Manager) Stop() error {
	logging.LogDebug(moduleName, "Stopping Resource Consumption Manager")

	if rcm.cancel != nil {
		rcm.cancel()
	}

	// Wait for all workers to finish
	rcm.wg.Wait()

	logging.LogDebug(moduleName, "Resource Consumption Manager stopped")
	return nil
}

// GetName returns the module name
func (rcm *Manager) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index
func (rcm *Manager) GetModuleIndex() int {
	return utils.ResourceConsumptionManager
}

// InstanceConfigUpdated updates limits when configuration changes
func (rcm *Manager) InstanceConfigUpdated() {
	rcm.mu.Lock()
	defer rcm.mu.Unlock()

	// Convert limits from config (GiB/MiB) to bytes
	rcm.diskLimit = int64(rcm.config.DiskLimit * 1_000_000_000) // GiB to bytes
	rcm.memoryLimit = int64(rcm.config.MemoryLimit * 1_000_000) // MiB to bytes
	rcm.cpuLimit = rcm.config.CPULimit                          // Percentage
}

// runUsageDataWorker periodically computes resource usage and updates status
func (rcm *Manager) runUsageDataWorker() {
	defer rcm.wg.Done()

	cfg := rcm.config
	ticker := time.NewTicker(time.Duration(cfg.GetUsageDataFreqSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-rcm.ctx.Done():
			return
		case <-ticker.C:
			rcm.collectUsageData()
		}
	}
}

// getMemoryUsage gets the memory usage of the ioFog process in bytes
func (rcm *Manager) getMemoryUsage() int64 {
	logging.LogDebug(moduleName, "Start get memory usage")

	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	// Go equivalent: Sys (total memory obtained from OS) - HeapIdle (free heap memory)
	// But more accurate: Alloc (allocated heap) + (Sys - HeapSys) (non-heap memory)
	memoryUsage := int64(m.Alloc) // #nosec G115 -- Alloc is heap bytes; practical values fit in int64

	logging.LogDebug(moduleName, fmt.Sprintf("Finished get memory usage: %d bytes (Alloc=%d, Sys=%d, HeapSys=%d, HeapIdle=%d)",
		memoryUsage, m.Alloc, m.Sys, m.HeapSys, m.HeapIdle))
	return memoryUsage
}

// getCPUUsage gets the CPU usage percentage of the ioFog process
func (rcm *Manager) getCPUUsage() float64 {
	logging.LogDebug(moduleName, "Start get cpu usage")

	// Get current process
	proc, err := process.NewProcess(int32(os.Getpid())) // #nosec G115 -- PID fits in int32 on all supported platforms
	if err != nil {
		logging.LogError(moduleName, "Error getting current process", err)
		return 0.0
	}

	// Get CPU percentage for this process
	cpuPercent, err := proc.Percent(time.Second)
	if err != nil {
		logging.LogError(moduleName, "Error getting CPU usage", err)
		return 0.0
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Finished get cpu usage: %.2f", cpuPercent))
	return cpuPercent
}

// getSystemAvailableMemory gets the system available memory in bytes
func (rcm *Manager) getSystemAvailableMemory() int64 {
	logging.LogDebug(moduleName, "Start get system available memory")

	vmStat, err := mem.VirtualMemory()
	if err != nil {
		logging.LogError(moduleName, "Error getting system available memory", err)
		return 0
	}

	availableMemory := int64(vmStat.Available) // #nosec G115 -- system available memory is below int64 max in practice
	logging.LogDebug(moduleName, fmt.Sprintf("Finished get system available memory: %d", availableMemory))
	return availableMemory
}

// getTotalCPU gets the total system CPU usage percentage
// Uses gopsutil cpu.Percent() for cross-platform support (Linux, Windows, macOS, etc.)
// This is the recommended method that works on all platforms
func (rcm *Manager) getTotalCPU() float64 {
	logging.LogDebug(moduleName, "Start get total cpu")

	// Use gopsutil cpu.Percent() - the recommended cross-platform method
	// When interval > 0, it blocks for that duration and measures CPU usage
	// percpu=false returns overall CPU usage (single value)
	percentages, err := cpu.Percent(1*time.Second, false)
	if err != nil {
		logging.LogError(moduleName, "Error getting total CPU usage", err)
		// Fallback to Linux-specific method if on Linux
		if runtime.GOOS == "linux" {
			logging.LogDebug(moduleName, "Falling back to Linux /proc/stat method")
			return rcm.getTotalCPULinux()
		}
		return 0.0
	}

	if len(percentages) == 0 {
		logging.LogWarn(moduleName, "No CPU percentage returned from gopsutil")
		// Fallback to Linux-specific method if on Linux
		if runtime.GOOS == "linux" {
			return rcm.getTotalCPULinux()
		}
		return 0.0
	}

	totalCPU := percentages[0]
	logging.LogDebug(moduleName, fmt.Sprintf("Finished get total cpu: %.2f%%", totalCPU))
	return totalCPU
}

// getTotalCPULinux reads /proc/stat to calculate total CPU usage
func (rcm *Manager) getTotalCPULinux() float64 {
	// Read /proc/stat
	statFile := "/proc/stat"
	data, err := os.ReadFile(statFile)
	if err != nil {
		logging.LogError(moduleName, "Error reading /proc/stat", err)
		return 0.0
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 {
		return 0.0
	}

	// Parse first line (cpu line)
	firstLine := strings.TrimSpace(lines[0])
	if !strings.HasPrefix(firstLine, "cpu ") {
		return 0.0
	}

	parts := strings.Fields(firstLine)
	if len(parts) < 8 {
		return 0.0
	}

	// Parse CPU times
	user, _ := strconv.ParseInt(parts[1], 10, 64)
	nice, _ := strconv.ParseInt(parts[2], 10, 64)
	system, _ := strconv.ParseInt(parts[3], 10, 64)
	idle, _ := strconv.ParseInt(parts[4], 10, 64)
	iowait, _ := strconv.ParseInt(parts[5], 10, 64)
	irq, _ := strconv.ParseInt(parts[6], 10, 64)
	softirq, _ := strconv.ParseInt(parts[7], 10, 64)
	steal := int64(0)
	if len(parts) >= 9 {
		steal, _ = strconv.ParseInt(parts[8], 10, 64)
	}

	totalTime := user + nice + system + idle + iowait + irq + softirq + steal
	idleTime := idle + iowait

	if totalTime > 0 {
		cpuUsage := float64(totalTime-idleTime) / float64(totalTime) * 100.0
		logging.LogDebug(moduleName, fmt.Sprintf("Finished get total cpu: %.2f", cpuUsage))
		return cpuUsage
	}

	return 0.0
}

// getAvailableDisk gets the available disk space in bytes
func (rcm *Manager) getAvailableDisk() int64 {
	logging.LogDebug(moduleName, "Start get available disk")

	cfg := rcm.config
	usage, err := disk.Usage(cfg.DiskDirectory)
	if err != nil {
		logging.LogError(moduleName, "Error getting available disk", err)
		return 0
	}

	availableDisk := int64(usage.Free) // #nosec G115 -- disk size is below int64 max in practice
	logging.LogDebug(moduleName, fmt.Sprintf("Finished get available disk: %d", availableDisk))
	return availableDisk
}

// getTotalDiskSpace gets the total disk space in bytes
func (rcm *Manager) getTotalDiskSpace() int64 {
	logging.LogDebug(moduleName, "Start get total disk space")

	cfg := rcm.config
	usage, err := disk.Usage(cfg.DiskDirectory)
	if err != nil {
		logging.LogError(moduleName, "Error getting total disk space", err)
		return 0
	}

	totalDiskSpace := int64(usage.Total) // #nosec G115 -- disk size is below int64 max in practice
	logging.LogDebug(moduleName, fmt.Sprintf("Finished get total disk space: %d", totalDiskSpace))
	return totalDiskSpace
}

// directorySize computes the size of a directory in bytes
func (rcm *Manager) directorySize(path string) int64 {
	logging.LogDebug(moduleName, fmt.Sprintf("Inside get directory size: %s", path))

	// Check if directory exists
	if _, err := os.Stat(path); os.IsNotExist(err) {
		logging.LogDebug(moduleName, fmt.Sprintf("Directory does not exist: %s, returning 0", path))
		return 0
	}

	var size int64
	err := filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			// Log but continue on error (e.g., permission denied)
			logging.LogDebug(moduleName, fmt.Sprintf("Error accessing file in %s: %v", path, err))
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	if err != nil {
		logging.LogError(moduleName, fmt.Sprintf("Error calculating directory size for %s", path), err)
		return 0
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Finished directory size: %d bytes for %s", size, path))
	return size
}

// removeArchives removes old archive files to free up disk space
func (rcm *Manager) removeArchives(amount int64) {
	logging.LogDebug(moduleName, fmt.Sprintf("Start remove archives: %d", amount))

	archivesDirectory := filepath.Join(rcm.config.DiskDirectory, "messages", "archive")

	// Get all .idx files
	files, err := filepath.Glob(filepath.Join(archivesDirectory, "*.idx"))
	if err != nil {
		logging.LogError(moduleName, "Error finding archive files", err)
		return
	}

	// Sort files by timestamp in filename (format: prefix_timestamp.idx)
	type fileInfo struct {
		path      string
		timestamp string
	}
	fileInfos := make([]fileInfo, 0, len(files))

	for _, file := range files {
		base := filepath.Base(file)
		// Extract timestamp from filename (format: prefix_timestamp.idx)
		ext := filepath.Ext(base)
		nameWithoutExt := base[:len(base)-len(ext)]
		parts := filepath.SplitList(nameWithoutExt)
		if len(parts) > 0 {
			lastPart := parts[len(parts)-1]
			// Try to find timestamp (after last underscore)
			idx := len(lastPart) - 1
			for idx >= 0 && lastPart[idx] != '_' {
				idx--
			}
			if idx >= 0 && idx < len(lastPart)-1 {
				timestamp := lastPart[idx+1:]
				fileInfos = append(fileInfos, fileInfo{
					path:      file,
					timestamp: timestamp,
				})
			}
		}
	}
	// Sort by timestamp (oldest first)
	slices.SortFunc(fileInfos, func(a, b fileInfo) int {
		return cmp.Compare(a.timestamp, b.timestamp)
	})

	// Remove files until we've freed enough space
	for _, fi := range fileInfos {
		if amount <= 0 {
			break
		}

		// Get file size
		info, err := os.Stat(fi.path)
		if err != nil {
			continue
		}
		idxSize := info.Size()

		// Remove .idx file
		if err := os.Remove(fi.path); err != nil {
			logging.LogError(moduleName, fmt.Sprintf("Error removing archive file %s", fi.path), err)
			continue
		}
		amount -= idxSize

		// Remove corresponding .iomsg file
		dataFile := fi.path[:len(fi.path)-len(".idx")] + ".iomsg"
		info, err = os.Stat(dataFile)
		if err == nil {
			dataSize := info.Size()
			if err := os.Remove(dataFile); err != nil {
				logging.LogError(moduleName, fmt.Sprintf("Error removing archive data file %s", dataFile), err)
			} else {
				amount -= dataSize
			}
		}
	}

	logging.LogDebug(moduleName, "Finished remove archives")
}
