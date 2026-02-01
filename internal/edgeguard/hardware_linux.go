// +build linux

package edgeguard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

// collectSystemInfo collects system/motherboard/BIOS info from /sys/class/dmi/id
func collectSystemInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== System Hardware ===\n")

	// System information from /sys/class/dmi/id
	dmiBase := "/sys/class/dmi/id"
	
	// System info
	data.WriteString("System:\n")
	if manufacturer := readDMIFile(dmiBase, "sys_vendor"); manufacturer != "" {
		data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", manufacturer))
	}
	if model := readDMIFile(dmiBase, "product_name"); model != "" {
		data.WriteString(fmt.Sprintf("  Model: %s\n", model))
	}
	if serial := readDMIFile(dmiBase, "product_serial"); serial != "" {
		data.WriteString(fmt.Sprintf("  Serial: %s\n", serial))
	}
	if uuid := readDMIFile(dmiBase, "product_uuid"); uuid != "" {
		data.WriteString(fmt.Sprintf("  UUID: %s\n", uuid))
	}

	// Motherboard info
	data.WriteString("Motherboard:\n")
	if mbManufacturer := readDMIFile(dmiBase, "board_vendor"); mbManufacturer != "" {
		data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", mbManufacturer))
	}
	if mbModel := readDMIFile(dmiBase, "board_name"); mbModel != "" {
		data.WriteString(fmt.Sprintf("  Model: %s\n", mbModel))
	}
	if mbVersion := readDMIFile(dmiBase, "board_version"); mbVersion != "" {
		data.WriteString(fmt.Sprintf("  Version: %s\n", mbVersion))
	}
	if mbSerial := readDMIFile(dmiBase, "board_serial"); mbSerial != "" {
		data.WriteString(fmt.Sprintf("  Serial: %s\n", mbSerial))
	}

	// BIOS/UEFI info
	data.WriteString("BIOS/UEFI:\n")
	if biosManufacturer := readDMIFile(dmiBase, "bios_vendor"); biosManufacturer != "" {
		data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", biosManufacturer))
	}

	return data.String()
}

// collectUsbInfo collects USB device information from /sys/bus/usb/devices
func collectUsbInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== USB Devices ===\n")

	// Check if we're in a container
	isContainer := strings.ToLower(os.Getenv("IOFOG_DAEMON")) == "container"
	if isContainer {
		logging.LogDebug(moduleName, "Running in container, filtering USB devices")
	}

	usbDir := "/sys/bus/usb/devices"
	entries, err := os.ReadDir(usbDir)
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error reading USB directory: %v", err))
		return data.String()
	}

	// First, identify USB controllers
	controllers := make(map[string]string)
	for _, entry := range entries {
		deviceName := entry.Name()
		if strings.HasPrefix(deviceName, "usb") {
			vendorID := readSysFile(filepath.Join(usbDir, deviceName, "idVendor"))
			productID := readSysFile(filepath.Join(usbDir, deviceName, "idProduct"))
			if vendorID != "" && productID != "" {
				controllers[deviceName] = fmt.Sprintf("%s:%s", vendorID, productID)
			}
		}
	}

	// Process all devices
	for _, entry := range entries {
		deviceName := entry.Name()
		// Skip USB controllers themselves
		if strings.HasPrefix(deviceName, "usb") {
			continue
		}

		devicePath := filepath.Join(usbDir, deviceName)
		vendorID := readSysFile(filepath.Join(devicePath, "idVendor"))
		productID := readSysFile(filepath.Join(devicePath, "idProduct"))

		if vendorID == "" || productID == "" {
			continue
		}

		manufacturer := readSysFile(filepath.Join(devicePath, "manufacturer"))
		product := readSysFile(filepath.Join(devicePath, "product"))
		serial := readSysFile(filepath.Join(devicePath, "serial"))
		speed := readSysFile(filepath.Join(devicePath, "speed"))
		deviceClass := readSysFile(filepath.Join(devicePath, "bDeviceClass"))

		// Check if this is a hub
		isHub := deviceClass == "09"

		// Skip hubs and virtual devices in containers
		if isHub || (isContainer && strings.HasPrefix(deviceName, "1-")) {
			continue
		}

		data.WriteString(fmt.Sprintf("  USB Device: %s\n", deviceName))
		data.WriteString(fmt.Sprintf("    Vendor ID: %s\n", vendorID))
		data.WriteString(fmt.Sprintf("    Product ID: %s\n", productID))
		if manufacturer != "" {
			data.WriteString(fmt.Sprintf("    Manufacturer: %s\n", manufacturer))
		}
		if product != "" {
			data.WriteString(fmt.Sprintf("    Product: %s\n", product))
		}
		if serial != "" {
			data.WriteString(fmt.Sprintf("    Serial: %s\n", serial))
		}
		if speed != "" {
			data.WriteString(fmt.Sprintf("    Speed: %s\n", speed))
		}
		if deviceClass != "" {
			data.WriteString(fmt.Sprintf("    Class: %s\n", deviceClass))
		}
	}

	return data.String()
}

// collectPciInfo collects PCI device information
func collectPciInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== PCI Devices ===\n")

	// Try /sys/bus/pci/devices first
	pciDir := "/sys/bus/pci/devices"
	entries, err := os.ReadDir(pciDir)
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error reading PCI directory: %v", err))
		return data.String()
	}

	// Graphics cards
	data.WriteString("Graphics Cards:\n")
	for _, entry := range entries {
		deviceName := entry.Name()
		devicePath := filepath.Join(pciDir, deviceName)
		
		// Check if it's a graphics card (class 0x03)
		class := readSysFile(filepath.Join(devicePath, "class"))
		if !strings.HasPrefix(class, "0x03") {
			continue
		}

		vendor := readSysFile(filepath.Join(devicePath, "vendor"))
		device := readSysFile(filepath.Join(devicePath, "device"))
		name := readSysFile(filepath.Join(devicePath, "uevent"))
		
		// Extract name from uevent if available
		if name != "" {
			lines := strings.Split(name, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PCI_NAME=") {
					name = strings.TrimPrefix(line, "PCI_NAME=")
					break
				}
			}
		}

		data.WriteString(fmt.Sprintf("  %s %s\n", deviceName, name))
		if vendor != "" {
			data.WriteString(fmt.Sprintf("    Vendor: %s\n", vendor))
		}
		if device != "" {
			data.WriteString(fmt.Sprintf("    Device: %s\n", device))
		}
	}

	// Sound cards (class 0x04)
	data.WriteString("Sound Cards:\n")
	for _, entry := range entries {
		deviceName := entry.Name()
		devicePath := filepath.Join(pciDir, deviceName)
		
		// Check if it's a sound card (class 0x04)
		class := readSysFile(filepath.Join(devicePath, "class"))
		if !strings.HasPrefix(class, "0x04") {
			continue
		}

		name := readSysFile(filepath.Join(devicePath, "uevent"))
		driver := readSysFile(filepath.Join(devicePath, "driver"))
		
		// Extract name from uevent if available
		if name != "" {
			lines := strings.Split(name, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "PCI_NAME=") {
					name = strings.TrimPrefix(line, "PCI_NAME=")
					break
				}
			}
		}

		data.WriteString(fmt.Sprintf("  %s %s\n", deviceName, name))
		if driver != "" {
			// Extract driver name from path
			driverName := filepath.Base(driver)
			data.WriteString(fmt.Sprintf("    Driver: %s\n", driverName))
		}
	}

	return data.String()
}

// collectStorageInfo collects detailed storage device info from /sys/block
func collectStorageInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Storage Devices ===\n")

	blockDir := "/sys/block"
	entries, err := os.ReadDir(blockDir)
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error reading block directory: %v", err))
		return data.String()
	}

	for _, entry := range entries {
		deviceName := entry.Name()
		
		// Skip virtual and special devices
		if strings.HasPrefix(deviceName, "vd") || // Virtual disks
			strings.HasPrefix(deviceName, "loop") || // Loop devices
			strings.HasPrefix(deviceName, "ram") || // RAM disks
			strings.HasPrefix(deviceName, "dm-") || // Device mapper
			strings.Contains(deviceName, "boot") { // Boot partitions
			continue
		}

		// Only process physical storage devices
		if strings.HasPrefix(deviceName, "sd") || // SATA/SCSI
			strings.HasPrefix(deviceName, "mmc") || // SD/eMMC
			strings.HasPrefix(deviceName, "nvme") || // NVMe
			strings.HasPrefix(deviceName, "hd") { // IDE
			
			devicePath := filepath.Join(blockDir, deviceName)
			model := readSysFile(filepath.Join(devicePath, "device/model"))
			vendor := readSysFile(filepath.Join(devicePath, "device/vendor"))
			serial := readSysFile(filepath.Join(devicePath, "device/serial"))
			size := readSysFile(filepath.Join(devicePath, "size"))
			removable := readSysFile(filepath.Join(devicePath, "removable"))

			data.WriteString(fmt.Sprintf("  Physical Storage: %s\n", deviceName))
			if model != "" {
				data.WriteString(fmt.Sprintf("    Model: %s\n", model))
			}
			if vendor != "" {
				data.WriteString(fmt.Sprintf("    Vendor: %s\n", vendor))
			}
			if serial != "" {
				data.WriteString(fmt.Sprintf("    Serial: %s\n", serial))
			}
			if size != "" {
				// Size is in 512-byte sectors
				var sizeInBytes int64
				fmt.Sscanf(size, "%d", &sizeInBytes)
				sizeInBytes *= 512
				data.WriteString(fmt.Sprintf("    Size: %d\n", sizeInBytes))
			}
			if removable != "" {
				removableStr := "No"
				if removable == "1" {
					removableStr = "Yes"
				}
				data.WriteString(fmt.Sprintf("    Removable: %s\n", removableStr))
			}
		}
	}

	return data.String()
}

// collectNetworkInfo collects detailed network interface info from /sys/class/net
func collectNetworkInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Network Interfaces ===\n")

	netDir := "/sys/class/net"
	entries, err := os.ReadDir(netDir)
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error reading network directory: %v", err))
		return data.String()
	}

	for _, entry := range entries {
		ifaceName := entry.Name()
		
		// Apply hybrid filtering: whitelist + blacklist (same as manager.go)
		// Always filter, not just in containers
		if !isPhysicalNetworkInterfaceLinux(ifaceName) {
			logging.LogDebug(moduleName, fmt.Sprintf("Filtered out virtual network interface: %s", ifaceName))
			continue
		}

		ifacePath := filepath.Join(netDir, ifaceName)
		carrier := readSysFile(filepath.Join(ifacePath, "carrier"))
		speed := readSysFile(filepath.Join(ifacePath, "speed"))
		duplex := readSysFile(filepath.Join(ifacePath, "duplex"))
		driver := readSysFile(filepath.Join(ifacePath, "device/driver"))
		
		// Extract driver name from path
		driverName := ""
		if driver != "" {
			driverName = filepath.Base(driver)
		}

		// Check if this is a WiFi interface and get SSID
		wifiSSID := ""
		if strings.HasPrefix(ifaceName, "wlan") || strings.HasPrefix(ifaceName, "wlp") {
			wifiSSID = getWiFiSSIDLinux(ifaceName)
		}

		data.WriteString(fmt.Sprintf("  Interface: %s\n", ifaceName))
		if carrier != "" {
			data.WriteString(fmt.Sprintf("    Carrier: %s\n", carrier))
		}
		if speed != "" && speed != "-1" {
			data.WriteString(fmt.Sprintf("    Speed: %s Mbps\n", speed))
		}
		if duplex != "" {
			data.WriteString(fmt.Sprintf("    Duplex: %s\n", duplex))
		}
		if driverName != "" {
			data.WriteString(fmt.Sprintf("    Driver: %s\n", driverName))
		}
		if wifiSSID != "" {
			data.WriteString(fmt.Sprintf("    WiFi SSID: %s\n", wifiSSID))
		}
	}

	return data.String()
}

// isPhysicalNetworkInterfaceLinux determines if a network interface is physical (not virtual) on Linux
// Uses hybrid approach: whitelist for common physical patterns + blacklist for known virtual patterns
func isPhysicalNetworkInterfaceLinux(name string) bool {
	// Blacklist: exclude known virtual interface patterns
	virtualPatterns := []string{
		"docker", "br-", "veth", "lo", "virbr", "vmnet", "tun", "tap",
	}
	for _, pattern := range virtualPatterns {
		if strings.HasPrefix(name, pattern) {
			return false
		}
	}

	// Whitelist: include known physical interface patterns
	physicalPatterns := []string{
		"eth", "en", "em", "wlan", "wlp",
	}
	for _, pattern := range physicalPatterns {
		if strings.HasPrefix(name, pattern) {
			return true
		}
	}

	// For interfaces not matching whitelist patterns, exclude them to be safe
	return false
}

// getWiFiSSIDLinux gets the WiFi SSID for a given interface on Linux
func getWiFiSSIDLinux(ifaceName string) string {
	// Try using iwgetid command first (most reliable)
	// iwgetid -r <interface> returns just the SSID
	cmd := exec.Command("iwgetid", "-r", ifaceName)
	if output, err := cmd.Output(); err == nil {
		ssid := strings.TrimSpace(string(output))
		if ssid != "" {
			return ssid
		}
	}

	// Fallback: try iw command
	cmd = exec.Command("iw", "dev", ifaceName, "info")
	if output, err := cmd.Output(); err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			if strings.Contains(line, "ssid") {
				parts := strings.Fields(line)
				for i, part := range parts {
					if part == "ssid" && i+1 < len(parts) {
						return strings.TrimSpace(parts[i+1])
					}
				}
			}
		}
	}

	return ""
}

// collectMemoryInfo collects physical memory module details from /sys/devices/system/node
func collectMemoryInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Physical Memory Modules ===\n")

	// Read memory info from /proc/meminfo for total
	meminfoPath := "/proc/meminfo"
	meminfo, err := os.ReadFile(meminfoPath)
	if err == nil {
		lines := strings.Split(string(meminfo), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				data.WriteString(fmt.Sprintf("  %s\n", strings.TrimSpace(line)))
				break
			}
		}
	}

	// Try to read memory module info from /sys/devices/system/node
	nodeDir := "/sys/devices/system/node"
	entries, err := os.ReadDir(nodeDir)
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "node") {
				nodePath := filepath.Join(nodeDir, entry.Name())
				meminfo := readSysFile(filepath.Join(nodePath, "meminfo"))
				if meminfo != "" {
					data.WriteString(fmt.Sprintf("  %s: %s\n", entry.Name(), meminfo))
				}
			}
		}
	}

	return data.String()
}

// readDMIFile reads a DMI file from /sys/class/dmi/id
func readDMIFile(basePath, filename string) string {
	path := filepath.Join(basePath, filename)
	return strings.TrimSpace(readSysFile(path))
}

// readSysFile reads a file from /sys filesystem
func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
