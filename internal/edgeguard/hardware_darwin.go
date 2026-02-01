// +build darwin

package edgeguard

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
)

// collectSystemInfo collects system/motherboard/BIOS info on macOS
func collectSystemInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== System Hardware ===\n")

	// Use system_profiler SPHardwareDataType for system info
	cmd := exec.Command("system_profiler", "SPHardwareDataType")
	output, err := cmd.Output()
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error running system_profiler: %v", err))
		// Fallback to ioreg
		return collectSystemInfoFromIOReg(ctx)
	}

	lines := strings.Split(string(output), "\n")
	
	// System info
	data.WriteString("System:\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "Model Name:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				data.WriteString(fmt.Sprintf("  Model: %s\n", strings.TrimSpace(parts[1])))
			}
		} else if strings.Contains(line, "Model Identifier:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				data.WriteString(fmt.Sprintf("  Model Identifier: %s\n", strings.TrimSpace(parts[1])))
			}
		} else if strings.Contains(line, "Serial Number") && !strings.Contains(line, "Board") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				data.WriteString(fmt.Sprintf("  Serial: %s\n", strings.TrimSpace(parts[1])))
			}
		} else if strings.Contains(line, "Hardware UUID:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				data.WriteString(fmt.Sprintf("  UUID: %s\n", strings.TrimSpace(parts[1])))
			}
		}
	}

	// Motherboard info (from ioreg)
	data.WriteString("Motherboard:\n")
	cmd = exec.Command("ioreg", "-c", "IOPlatformExpertDevice", "-d", "2")
	output, err = cmd.Output()
	if err == nil {
		outputStr := string(output)
		if manufacturer := extractIORegValue(outputStr, "manufacturer"); manufacturer != "" {
			data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", manufacturer))
		}
		if model := extractIORegValue(outputStr, "board-id"); model != "" {
			data.WriteString(fmt.Sprintf("  Model: %s\n", model))
		}
	}

	// BIOS/UEFI info (macOS doesn't have traditional BIOS, but has firmware)
	data.WriteString("BIOS/UEFI:\n")
	cmd = exec.Command("system_profiler", "SPHardwareDataType")
	output, err = cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "Boot ROM Version:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					data.WriteString(fmt.Sprintf("  Version: %s\n", strings.TrimSpace(parts[1])))
				}
			}
		}
	}

	return data.String()
}

// collectSystemInfoFromIOReg fallback method using ioreg
func collectSystemInfoFromIOReg(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== System Hardware ===\n")

	cmd := exec.Command("ioreg", "-c", "IOPlatformExpertDevice", "-d", "2")
	output, err := cmd.Output()
	if err != nil {
		return data.String()
	}

	outputStr := string(output)
	
	data.WriteString("System:\n")
	if serial := extractIORegValue(outputStr, "IOPlatformSerialNumber"); serial != "" {
		data.WriteString(fmt.Sprintf("  Serial: %s\n", serial))
	}
	if uuid := extractIORegValue(outputStr, "IOPlatformUUID"); uuid != "" {
		data.WriteString(fmt.Sprintf("  UUID: %s\n", uuid))
	}
	if model := extractIORegValue(outputStr, "model"); model != "" {
		data.WriteString(fmt.Sprintf("  Model: %s\n", model))
	}

	return data.String()
}

// extractIORegValue extracts a value from ioreg output
func extractIORegValue(output, key string) string {
	// Look for pattern: "key" = "value"
	pattern := fmt.Sprintf(`"%s"\s*=\s*"([^"]+)"`, key)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// collectUsbInfo collects USB device information on macOS
func collectUsbInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== USB Devices ===\n")

	// Use system_profiler SPUSBDataType for USB devices
	cmd := exec.Command("system_profiler", "SPUSBDataType")
	output, err := cmd.Output()
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error running system_profiler SPUSBDataType: %v", err))
		// Fallback to ioreg
		return collectUsbInfoFromIOReg(ctx)
	}

	lines := strings.Split(string(output), "\n")
	var currentDevice strings.Builder
	inDevice := false
	deviceLevel := 0

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " ")
		level := (len(line) - len(trimmed)) / 2
		
		if strings.HasPrefix(trimmed, "USB") || strings.HasPrefix(trimmed, "Product ID:") {
			if inDevice && currentDevice.Len() > 0 {
				data.WriteString(fmt.Sprintf("  USB Device:\n%s", currentDevice.String()))
				currentDevice.Reset()
			}
			inDevice = true
			deviceLevel = level
			continue
		}
		
		if inDevice && level > deviceLevel {
			if strings.Contains(trimmed, ":") {
				parts := strings.SplitN(trimmed, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if value != "" {
						// Map to standard format
						switch key {
						case "Product ID":
							currentDevice.WriteString(fmt.Sprintf("    Product ID: %s\n", value))
						case "Vendor ID":
							currentDevice.WriteString(fmt.Sprintf("    Vendor ID: %s\n", value))
						case "Manufacturer":
							currentDevice.WriteString(fmt.Sprintf("    Manufacturer: %s\n", value))
						case "Product Name", "Product":
							currentDevice.WriteString(fmt.Sprintf("    Product: %s\n", value))
						case "Serial Number", "Serial":
							currentDevice.WriteString(fmt.Sprintf("    Serial: %s\n", value))
						}
					}
				}
			}
		} else if inDevice && level <= deviceLevel {
			// End of current device
			if currentDevice.Len() > 0 {
				data.WriteString(fmt.Sprintf("  USB Device:\n%s", currentDevice.String()))
				currentDevice.Reset()
			}
			inDevice = false
		}
	}
	if inDevice && currentDevice.Len() > 0 {
		data.WriteString(fmt.Sprintf("  USB Device:\n%s", currentDevice.String()))
	}

	return data.String()
}

// collectUsbInfoFromIOReg fallback method using ioreg
func collectUsbInfoFromIOReg(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== USB Devices ===\n")

	cmd := exec.Command("ioreg", "-p", "IOUSB", "-w0")
	output, err := cmd.Output()
	if err != nil {
		return data.String()
	}

	// Parse USB devices from ioreg output
	// This is a simplified version
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.Contains(line, "USB") && strings.Contains(line, "Product Name") {
			// Extract USB device info
			if productName := extractIORegValue(line, "Product Name"); productName != "" {
				data.WriteString(fmt.Sprintf("  USB Device: %s\n", productName))
			}
		}
	}

	return data.String()
}

// collectPciInfo collects PCI device information on macOS
func collectPciInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== PCI Devices ===\n")

	// Use system_profiler SPDisplaysDataType for graphics cards
	cmd := exec.Command("system_profiler", "SPDisplaysDataType")
	output, err := cmd.Output()
	if err == nil {
		lines := strings.Split(string(output), "\n")
		data.WriteString("Graphics Cards:\n")
		inDisplay := false
		var currentDisplay strings.Builder
		
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Display Type:") || strings.HasPrefix(line, "Chipset Model:") {
				if inDisplay && currentDisplay.Len() > 0 {
					data.WriteString(fmt.Sprintf("  %s", currentDisplay.String()))
					currentDisplay.Reset()
				}
				inDisplay = true
			}
			if inDisplay {
				if strings.Contains(line, ":") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						key := strings.TrimSpace(parts[0])
						value := strings.TrimSpace(parts[1])
						if value != "" {
							currentDisplay.WriteString(fmt.Sprintf("    %s: %s\n", key, value))
						}
					}
				}
			}
		}
		if inDisplay && currentDisplay.Len() > 0 {
			data.WriteString(fmt.Sprintf("  %s", currentDisplay.String()))
		}
	}

	// Use ioreg for PCI devices
	cmd = exec.Command("ioreg", "-p", "IODeviceTree", "-n", "pci-bridge", "-r")
	output, err = cmd.Output()
	if err == nil {
		// Parse PCI devices from ioreg output
		// This is a simplified version - can be enhanced
		if strings.Contains(string(output), "pci") {
			data.WriteString("PCI Devices:\n")
			data.WriteString("  (PCI device enumeration from ioreg)\n")
		}
	}

	return data.String()
}

// collectStorageInfo collects detailed storage device info on macOS
func collectStorageInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Storage Devices ===\n")

	// Use diskutil list to get physical disks
	cmd := exec.Command("diskutil", "list", "-plist")
	output, err := cmd.Output()
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error running diskutil: %v", err))
		return data.String()
	}

	// Parse diskutil output or use system_profiler for more details
	cmd = exec.Command("system_profiler", "SPStorageDataType")
	output, err = cmd.Output()
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error running system_profiler SPStorageDataType: %v", err))
		return data.String()
	}

	lines := strings.Split(string(output), "\n")
	var currentDevice strings.Builder
	inDevice := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Storage:") || strings.HasPrefix(line, "Physical Store:") {
			if inDevice && currentDevice.Len() > 0 {
				data.WriteString(fmt.Sprintf("  Storage Device:\n%s", currentDevice.String()))
				currentDevice.Reset()
			}
			inDevice = true
			continue
		}
		if inDevice {
			if strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if value != "" {
						// Map keys to standard format
						switch key {
						case "Media Name", "Device Name":
							currentDevice.WriteString(fmt.Sprintf("    Model: %s\n", value))
						case "Serial Number", "Volume UUID":
							if key == "Serial Number" {
								currentDevice.WriteString(fmt.Sprintf("    Serial: %s\n", value))
							}
						case "Size", "Capacity":
							currentDevice.WriteString(fmt.Sprintf("    Size: %s\n", value))
						case "Media Type":
							currentDevice.WriteString(fmt.Sprintf("    Type: %s\n", value))
						case "Removable Media":
							currentDevice.WriteString(fmt.Sprintf("    Removable: %s\n", value))
						}
					}
				}
			}
		}
	}
	if inDevice && currentDevice.Len() > 0 {
		data.WriteString(fmt.Sprintf("  Storage Device:\n%s", currentDevice.String()))
	}

	return data.String()
}

// collectNetworkInfo collects detailed network interface info on macOS
func collectNetworkInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Network Interfaces ===\n")

	// Get list of network interfaces using ifconfig
	cmd := exec.Command("ifconfig", "-l")
	output, err := cmd.Output()
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error running ifconfig -l: %v", err))
		return data.String()
	}

	interfaces := strings.Fields(string(output))
	
	for _, ifaceName := range interfaces {
		// Apply hybrid filtering: whitelist + blacklist
		if !isPhysicalNetworkInterfaceDarwin(ifaceName) {
			logging.LogDebug(moduleName, fmt.Sprintf("Filtered out virtual network interface: %s", ifaceName))
			continue
		}

		// Get interface details using ifconfig
		cmd = exec.Command("ifconfig", ifaceName)
		ifconfigOutput, err := cmd.Output()
		if err != nil {
			continue
		}

		// Parse MAC address
		macAddr := extractMACFromIfconfig(string(ifconfigOutput))
		
		// Get link state and speed
		carrier := "unknown"
		speed := "unknown"
		if strings.Contains(string(ifconfigOutput), "status: active") {
			carrier = "1"
		} else if strings.Contains(string(ifconfigOutput), "status: inactive") {
			carrier = "0"
		}

		// Try to get speed from ifconfig or networksetup
		cmd = exec.Command("networksetup", "-getmedia", ifaceName)
		if mediaOutput, err := cmd.Output(); err == nil {
			mediaStr := string(mediaOutput)
			if strings.Contains(mediaStr, "1000baseT") {
				speed = "1000"
			} else if strings.Contains(mediaStr, "100baseT") {
				speed = "100"
			} else if strings.Contains(mediaStr, "10baseT") {
				speed = "10"
			}
		}

		// Get WiFi SSID if this is a WiFi interface
		wifiSSID := ""
		if strings.HasPrefix(ifaceName, "en") && (strings.Contains(ifaceName, "0") || strings.Contains(ifaceName, "1")) {
			// Try to get WiFi SSID using networksetup
			cmd = exec.Command("networksetup", "-getairportnetwork", ifaceName)
			if ssidOutput, err := cmd.Output(); err == nil {
				ssidStr := strings.TrimSpace(string(ssidOutput))
				if !strings.Contains(ssidStr, "Error") && ssidStr != "" {
					// Format: "Current Wi-Fi Network: SSID" or just "SSID"
					if strings.Contains(ssidStr, ":") {
						parts := strings.SplitN(ssidStr, ":", 2)
						if len(parts) == 2 {
							wifiSSID = strings.TrimSpace(parts[1])
						}
					} else {
						wifiSSID = ssidStr
					}
				}
			}
			// Fallback: try airport command
			if wifiSSID == "" {
				wifiSSID = getWiFiSSIDDarwin(ifaceName)
			}
		}

		data.WriteString(fmt.Sprintf("  Interface: %s\n", ifaceName))
		if macAddr != "" {
			data.WriteString(fmt.Sprintf("    MAC Address: %s\n", macAddr))
		}
		if carrier != "unknown" {
			data.WriteString(fmt.Sprintf("    Carrier: %s\n", carrier))
		}
		if speed != "unknown" {
			data.WriteString(fmt.Sprintf("    Speed: %s Mbps\n", speed))
		}
		if wifiSSID != "" {
			data.WriteString(fmt.Sprintf("    WiFi SSID: %s\n", wifiSSID))
		}
	}

	return data.String()
}

// isPhysicalNetworkInterfaceDarwin determines if a network interface is physical (not virtual) on macOS
func isPhysicalNetworkInterfaceDarwin(name string) bool {
	// Blacklist: exclude known virtual interface patterns
	virtualPatterns := []string{
		"bridge", "veth", "lo0", "gif", "stf", "utun", "awdl", "llw", "anpi", "ap1",
	}
	for _, pattern := range virtualPatterns {
		if strings.HasPrefix(name, pattern) {
			return false
		}
	}

	// Whitelist: include known physical interface patterns
	physicalPatterns := []string{
		"en", "wlan", "airport",
	}
	for _, pattern := range physicalPatterns {
		if strings.HasPrefix(name, pattern) {
			return true
		}
	}

	// For interfaces not matching whitelist patterns, exclude them to be safe
	return false
}

// extractMACFromIfconfig extracts MAC address from ifconfig output
func extractMACFromIfconfig(output string) string {
	// Look for pattern: ether XX:XX:XX:XX:XX:XX
	re := regexp.MustCompile(`ether\s+([0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2})`)
	matches := re.FindStringSubmatch(output)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// getWiFiSSIDDarwin gets WiFi SSID using airport command
func getWiFiSSIDDarwin(ifaceName string) string {
	// Try using airport command (requires symlink or full path)
	airportPath := "/System/Library/PrivateFrameworks/Apple80211.framework/Resources/airport"
	cmd := exec.Command(airportPath, "-I")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	// Parse SSID from airport output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, " SSID:") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// collectMemoryInfo collects physical memory module details on macOS
func collectMemoryInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Physical Memory Modules ===\n")

	// Use system_profiler SPMemoryDataType for memory module info
	cmd := exec.Command("system_profiler", "SPMemoryDataType")
	output, err := cmd.Output()
	if err != nil {
		logging.LogDebug(moduleName, fmt.Sprintf("Error running system_profiler SPMemoryDataType: %v", err))
		// Fallback: get total memory from sysctl
		cmd = exec.Command("sysctl", "hw.memsize")
		if output, err := cmd.Output(); err == nil {
			parts := strings.Fields(string(output))
			if len(parts) > 1 {
				data.WriteString(fmt.Sprintf("  Total Memory: %s bytes\n", parts[1]))
			}
		}
		return data.String()
	}

	lines := strings.Split(string(output), "\n")
	var currentModule strings.Builder
	inModule := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Memory Slots:") {
			// Skip header
			continue
		}
		if strings.HasPrefix(line, "BANK") || strings.HasPrefix(line, "DIMM") {
			if inModule && currentModule.Len() > 0 {
				data.WriteString(fmt.Sprintf("  Memory Module:\n%s", currentModule.String()))
				currentModule.Reset()
			}
			inModule = true
			continue
		}
		if inModule {
			if strings.Contains(line, ":") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					if value != "" && value != "Empty" {
						currentModule.WriteString(fmt.Sprintf("    %s: %s\n", key, value))
					}
				}
			}
		}
	}
	if inModule && currentModule.Len() > 0 {
		data.WriteString(fmt.Sprintf("  Memory Module:\n%s", currentModule.String()))
	}

	return data.String()
}
