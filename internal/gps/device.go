package gps

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/datasance/edgelet/internal/config"
	"github.com/datasance/edgelet/internal/gps/nmea"
	"github.com/datasance/edgelet/internal/utils/logging"
	"golang.org/x/sys/unix"
)

const (
	deviceHandlerModuleName = "GPS Device Handler"
	readTimeout             = 5 * time.Second
)

// DeviceHandler handles GPS device communication
type DeviceHandler struct {
	manager    *Manager
	config     *config.Config
	devicePath string
	deviceFile *os.File
	reader     *bufio.Reader
	isRunning  bool
	mu         sync.RWMutex
}

// NewDeviceHandler creates a new DeviceHandler
func NewDeviceHandler(manager *Manager) *DeviceHandler {
	return &DeviceHandler{
		manager: manager,
		config:  config.GetInstance(),
	}
}

// Start starts the device handler
func (d *DeviceHandler) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.isRunning {
		return nil
	}

	devicePath := d.config.GPSDevice
	if devicePath == "" {
		return fmt.Errorf("GPS device not configured")
	}

	logging.LogDebug(deviceHandlerModuleName, fmt.Sprintf("Starting GPS device handler: %s", devicePath))

	// Open device file
	file, err := os.OpenFile(filepath.Clean(devicePath), os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("failed to open GPS device: %w", err)
	}

	d.devicePath = devicePath
	d.deviceFile = file
	d.reader = bufio.NewReader(file)
	d.isRunning = true

	logging.LogInfo(deviceHandlerModuleName, fmt.Sprintf("GPS device handler started: %s", devicePath))
	return nil
}

// Stop stops the device handler
func (d *DeviceHandler) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.isRunning {
		return nil
	}

	logging.LogDebug(deviceHandlerModuleName, "Stopping GPS device handler")

	if d.deviceFile != nil {
		if err := d.deviceFile.Close(); err != nil {
			logging.LogWarn(deviceHandlerModuleName, fmt.Sprintf("Failed to close GPS device file: %v", err))
		}
		d.deviceFile = nil
	}
	d.reader = nil
	d.isRunning = false

	logging.LogInfo(deviceHandlerModuleName, "GPS device handler stopped")
	return nil
}

// IsRunning returns whether the device handler is running
func (d *DeviceHandler) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.isRunning
}

// ReadAndUpdateCoordinates reads from GPS device and updates coordinates if valid
func (d *DeviceHandler) ReadAndUpdateCoordinates() error {
	d.mu.RLock()
	if !d.isRunning || d.reader == nil {
		d.mu.RUnlock()
		return fmt.Errorf("device handler not running")
	}
	reader := d.reader
	deviceFile := d.deviceFile
	d.mu.RUnlock()
	if deviceFile == nil {
		return fmt.Errorf("device file is not open")
	}

	// Read line with timeout
	line, err := d.readLineWithTimeout(deviceFile, reader, readTimeout)
	if err != nil {
		logging.LogWarn(deviceHandlerModuleName, fmt.Sprintf("GPS device timeout - no data received within %v", readTimeout))
		return err
	}

	// Parse NMEA message
	nmeaMsg, err := nmea.Parse(line)
	if err != nil {
		logging.LogWarn(deviceHandlerModuleName, fmt.Sprintf("Failed to parse NMEA message: %v", err))
		return err
	}

	if !nmeaMsg.IsValid() {
		logging.LogWarn(deviceHandlerModuleName, "Invalid NMEA message received")
		return fmt.Errorf("invalid NMEA message")
	}

	// Update coordinates in configuration
	coordinates := fmt.Sprintf("%.5f,%.5f", nmeaMsg.GetLatitude(), nmeaMsg.GetLongitude())
	d.config.GPSCoordinates = coordinates

	logging.LogDebug(deviceHandlerModuleName, fmt.Sprintf("Updated GPS coordinates: %s", coordinates))
	return nil
}

// readLineWithTimeout reads a line from reader with timeout
func (d *DeviceHandler) readLineWithTimeout(file *os.File, reader *bufio.Reader, timeout time.Duration) (string, error) {
	if file == nil {
		return "", fmt.Errorf("nil device file")
	}

	pollFds := []unix.PollFd{{
		Fd:     int32(file.Fd()),
		Events: unix.POLLIN | unix.POLLPRI,
	}}
	pollTimeoutMs := int(timeout.Milliseconds())
	if pollTimeoutMs < 0 {
		pollTimeoutMs = -1
	}

	n, err := unix.Poll(pollFds, pollTimeoutMs)
	if err != nil {
		return "", fmt.Errorf("poll failed: %w", err)
	}
	if n == 0 {
		return "", fmt.Errorf("read timeout")
	}
	if pollFds[0].Revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
		return "", errors.New("device poll error")
	}
	if pollFds[0].Revents&(unix.POLLIN|unix.POLLPRI) == 0 {
		return "", errors.New("device not readable")
	}

	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}
