//go:build windows
// +build windows

package edgeguard

import (
	"context"
	"fmt"
	"strings"

	"github.com/eclipse-iofog/agent/internal/utils/logging"
	"github.com/yusufpapurcu/wmi"
)

// collectSystemInfo collects system/motherboard/BIOS info on Windows
func collectSystemInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== System Hardware ===\n")

	// System info from Win32_ComputerSystem
	type ComputerSystem struct {
		Manufacturer string
		Model        string
	}
	var systems []ComputerSystem
	if err := wmi.Query("SELECT Manufacturer, Model FROM Win32_ComputerSystem", &systems); err == nil && len(systems) > 0 {
		sys := systems[0]
		data.WriteString("System:\n")
		if sys.Manufacturer != "" {
			data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", sys.Manufacturer))
		}
		if sys.Model != "" {
			data.WriteString(fmt.Sprintf("  Model: %s\n", sys.Model))
		}
	}

	// System UUID/Serial from Win32_ComputerSystemProduct
	type ComputerSystemProduct struct {
		UUID string
	}
	var products []ComputerSystemProduct
	if err := wmi.Query("SELECT UUID FROM Win32_ComputerSystemProduct", &products); err == nil && len(products) > 0 {
		if products[0].UUID != "" {
			data.WriteString(fmt.Sprintf("  UUID: %s\n", products[0].UUID))
		}
	}

	// Motherboard info from Win32_BaseBoard
	type BaseBoard struct {
		Manufacturer string
		Product      string
		Version      string
		SerialNumber string
	}
	var boards []BaseBoard
	if err := wmi.Query("SELECT Manufacturer, Product, Version, SerialNumber FROM Win32_BaseBoard", &boards); err == nil && len(boards) > 0 {
		board := boards[0]
		data.WriteString("Motherboard:\n")
		if board.Manufacturer != "" {
			data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", board.Manufacturer))
		}
		if board.Product != "" {
			data.WriteString(fmt.Sprintf("  Model: %s\n", board.Product))
		}
		if board.Version != "" {
			data.WriteString(fmt.Sprintf("  Version: %s\n", board.Version))
		}
		if board.SerialNumber != "" {
			data.WriteString(fmt.Sprintf("  Serial: %s\n", board.SerialNumber))
		}
	}

	// BIOS info from Win32_BIOS
	type BIOS struct {
		Manufacturer string
		Version      string
		ReleaseDate  string
	}
	var biosList []BIOS
	if err := wmi.Query("SELECT Manufacturer, Version, ReleaseDate FROM Win32_BIOS", &biosList); err == nil && len(biosList) > 0 {
		bios := biosList[0]
		data.WriteString("BIOS/UEFI:\n")
		if bios.Manufacturer != "" {
			data.WriteString(fmt.Sprintf("  Manufacturer: %s\n", bios.Manufacturer))
		}
		if bios.Version != "" {
			data.WriteString(fmt.Sprintf("  Version: %s\n", bios.Version))
		}
		if bios.ReleaseDate != "" {
			data.WriteString(fmt.Sprintf("  Date: %s\n", bios.ReleaseDate))
		}
	}

	return data.String()
}

// collectUsbInfo collects USB device information on Windows
func collectUsbInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== USB Devices ===\n")

	// USB devices from Win32_PnPEntity filtered by USB
	type PnPEntity struct {
		Name         string
		DeviceID     string
		Manufacturer string
		Service      string
		Status       string
	}
	var pnpEntities []PnPEntity
	// Query for USB devices - DeviceID contains USB
	if err := wmi.Query("SELECT Name, DeviceID, Manufacturer, Service, Status FROM Win32_PnPEntity WHERE DeviceID LIKE 'USB%'", &pnpEntities); err == nil {
		for _, device := range pnpEntities {
			// Filter out virtual USB devices
			if strings.Contains(strings.ToLower(device.Name), "virtual") ||
				strings.Contains(strings.ToLower(device.Service), "virtual") {
				continue
			}

			// Extract Vendor ID and Product ID from DeviceID
			// Format: USB\\VID_XXXX&PID_XXXX\\...
			vendorID := ""
			productID := ""
			parts := strings.Split(device.DeviceID, "\\")
			if len(parts) > 1 {
				usbPart := parts[1]
				if strings.HasPrefix(usbPart, "VID_") {
					vidParts := strings.Split(usbPart, "&")
					if len(vidParts) > 0 {
						vendorID = strings.TrimPrefix(vidParts[0], "VID_")
					}
					if len(vidParts) > 1 && strings.HasPrefix(vidParts[1], "PID_") {
						productID = strings.TrimPrefix(vidParts[1], "PID_")
					}
				}
			}

			data.WriteString(fmt.Sprintf("  USB Device: %s\n", device.Name))
			if vendorID != "" {
				data.WriteString(fmt.Sprintf("    Vendor ID: %s\n", vendorID))
			}
			if productID != "" {
				data.WriteString(fmt.Sprintf("    Product ID: %s\n", productID))
			}
			if device.Manufacturer != "" {
				data.WriteString(fmt.Sprintf("    Manufacturer: %s\n", device.Manufacturer))
			}
			if device.Name != "" {
				// Extract product name from device name
				data.WriteString(fmt.Sprintf("    Product: %s\n", device.Name))
			}
		}
	} else {
		logging.LogDebug(moduleName, fmt.Sprintf("Error querying Win32_PnPEntity for USB: %v", err))
	}

	return data.String()
}

// collectPciInfo collects PCI device information on Windows
func collectPciInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== PCI Devices ===\n")

	// Graphics cards from Win32_VideoController
	type VideoController struct {
		Name          string
		AdapterRAM    uint64
		DriverVersion string
	}
	var videoControllers []VideoController
	if err := wmi.Query("SELECT Name, AdapterRAM, DriverVersion FROM Win32_VideoController", &videoControllers); err == nil {
		data.WriteString("Graphics Cards:\n")
		for _, vc := range videoControllers {
			if vc.Name != "" {
				data.WriteString(fmt.Sprintf("  %s\n", vc.Name))
				if vc.AdapterRAM > 0 {
					data.WriteString(fmt.Sprintf("    Memory: %d bytes\n", vc.AdapterRAM))
				}
				if vc.DriverVersion != "" {
					data.WriteString(fmt.Sprintf("    Driver Version: %s\n", vc.DriverVersion))
				}
			}
		}
	}

	// Sound cards from Win32_SoundDevice
	type SoundDevice struct {
		Name         string
		Manufacturer string
		ProductName  string
	}
	var soundDevices []SoundDevice
	if err := wmi.Query("SELECT Name, Manufacturer, ProductName FROM Win32_SoundDevice", &soundDevices); err == nil {
		data.WriteString("Sound Cards:\n")
		for _, sd := range soundDevices {
			if sd.Name != "" {
				data.WriteString(fmt.Sprintf("  %s\n", sd.Name))
				if sd.Manufacturer != "" {
					data.WriteString(fmt.Sprintf("    Manufacturer: %s\n", sd.Manufacturer))
				}
				if sd.ProductName != "" {
					data.WriteString(fmt.Sprintf("    Product: %s\n", sd.ProductName))
				}
			}
		}
	}

	return data.String()
}

// collectStorageInfo collects detailed storage device info on Windows
func collectStorageInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Storage Devices ===\n")

	// Physical disks from Win32_DiskDrive (includes removable storage)
	type DiskDrive struct {
		Model         string
		SerialNumber  string
		Size          uint64
		MediaType     string
		InterfaceType string
		DeviceID      string
	}
	var drives []DiskDrive
	if err := wmi.Query("SELECT Model, SerialNumber, Size, MediaType, InterfaceType, DeviceID FROM Win32_DiskDrive", &drives); err == nil {
		for _, drive := range drives {
			// Include all drives (including removable) as per requirements
			data.WriteString(fmt.Sprintf("  Storage Device: %s\n", drive.DeviceID))
			if drive.Model != "" {
				data.WriteString(fmt.Sprintf("    Model: %s\n", strings.TrimSpace(drive.Model)))
			}
			if drive.SerialNumber != "" {
				data.WriteString(fmt.Sprintf("    Serial: %s\n", drive.SerialNumber))
			}
			if drive.Size > 0 {
				data.WriteString(fmt.Sprintf("    Size: %d\n", drive.Size))
			}
			if drive.MediaType != "" {
				data.WriteString(fmt.Sprintf("    Type: %s\n", drive.MediaType))
			} else if drive.InterfaceType != "" {
				data.WriteString(fmt.Sprintf("    Type: %s\n", drive.InterfaceType))
			}
			// Determine if removable
			isRemovable := strings.Contains(strings.ToLower(drive.InterfaceType), "usb") ||
				strings.Contains(strings.ToLower(drive.MediaType), "removable") ||
				strings.Contains(strings.ToLower(drive.MediaType), "external")
			if isRemovable {
				data.WriteString("    Removable: Yes\n")
			} else {
				data.WriteString("    Removable: No\n")
			}
		}
	} else {
		logging.LogDebug(moduleName, fmt.Sprintf("Error querying Win32_DiskDrive: %v", err))
	}

	return data.String()
}

// collectNetworkInfo collects detailed network interface info on Windows
func collectNetworkInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Network Interfaces ===\n")

	// Network adapters from Win32_NetworkAdapter
	type NetworkAdapter struct {
		Name            string
		MACAddress      string
		AdapterTypeID   uint16
		PhysicalAdapter bool
		NetConnectionID string
		Speed           uint64
		Status          string
		DeviceID        string
	}
	var adapters []NetworkAdapter
	if err := wmi.Query("SELECT Name, MACAddress, AdapterTypeID, PhysicalAdapter, NetConnectionID, Speed, Status, DeviceID FROM Win32_NetworkAdapter", &adapters); err == nil {
		for _, adapter := range adapters {
			// Filter: include physical adapters, exclude virtual ones
			if !isPhysicalNetworkAdapterWindows(adapter) {
				logging.LogDebug(moduleName, fmt.Sprintf("Filtered out virtual network adapter: %s", adapter.Name))
				continue
			}

			// Skip if no MAC address or invalid MAC
			if adapter.MACAddress == "" || adapter.MACAddress == "00:00:00:00:00:00" {
				continue
			}

			data.WriteString(fmt.Sprintf("  Interface: %s\n", adapter.NetConnectionID))
			if adapter.Name != "" {
				data.WriteString(fmt.Sprintf("    Name: %s\n", adapter.Name))
			}
			if adapter.MACAddress != "" {
				data.WriteString(fmt.Sprintf("    MAC Address: %s\n", adapter.MACAddress))
			}
			if adapter.Speed > 0 {
				data.WriteString(fmt.Sprintf("    Speed: %d bps\n", adapter.Speed))
			}
			if adapter.Status != "" {
				carrier := "0"
				if strings.Contains(strings.ToLower(adapter.Status), "connected") || strings.Contains(strings.ToLower(adapter.Status), "up") {
					carrier = "1"
				}
				data.WriteString(fmt.Sprintf("    Carrier: %s\n", carrier))
			}

			// Get WiFi SSID if this is a wireless adapter
			if adapter.AdapterTypeID == 9 || strings.Contains(strings.ToLower(adapter.Name), "wireless") || strings.Contains(strings.ToLower(adapter.Name), "wifi") {
				wifiSSID := getWiFiSSIDWindows(adapter.NetConnectionID)
				if wifiSSID != "" {
					data.WriteString(fmt.Sprintf("    WiFi SSID: %s\n", wifiSSID))
				}
			}
		}
	} else {
		logging.LogDebug(moduleName, fmt.Sprintf("Error querying Win32_NetworkAdapter: %v", err))
	}

	return data.String()
}

// isPhysicalNetworkAdapterWindows determines if a network adapter is physical (not virtual) on Windows
func isPhysicalNetworkAdapterWindows(adapter NetworkAdapter) bool {
	// Exclude virtual adapters
	virtualKeywords := []string{
		"Hyper-V", "VirtualBox", "VMware", "Docker", "vEthernet", "WSL", "Virtual", "TAP", "Loopback",
	}
	netConnectionID := strings.ToLower(adapter.NetConnectionID)
	name := strings.ToLower(adapter.Name)

	for _, keyword := range virtualKeywords {
		if strings.Contains(netConnectionID, strings.ToLower(keyword)) || strings.Contains(name, strings.ToLower(keyword)) {
			return false
		}
	}

	// Include if PhysicalAdapter is true OR AdapterTypeID is not 0 (unknown/virtual)
	if adapter.PhysicalAdapter {
		return true
	}
	if adapter.AdapterTypeID != 0 {
		return true
	}

	// Exclude if both are false/0
	return false
}

// getWiFiSSIDWindows gets WiFi SSID for a network adapter on Windows
func getWiFiSSIDWindows(netConnectionID string) string {
	// Query Win32_NetworkAdapterConfiguration for SSID
	type NetworkAdapterConfig struct {
		Description string
		SettingID   string
	}
	var configs []NetworkAdapterConfig
	query := fmt.Sprintf("SELECT Description, SettingID FROM Win32_NetworkAdapterConfiguration WHERE Description='%s'", netConnectionID)
	if err := wmi.Query(query, &configs); err != nil {
		return ""
	}

	// SSID is typically stored in registry or can be retrieved via netsh
	// For now, return empty - can be enhanced with registry query or netsh command
	// netsh wlan show interfaces | findstr SSID
	return ""
}

// collectMemoryInfo collects physical memory module details on Windows
func collectMemoryInfo(ctx context.Context) string {
	var data strings.Builder
	data.WriteString("\n=== Physical Memory Modules ===\n")

	// Memory modules from Win32_PhysicalMemory
	type PhysicalMemory struct {
		Capacity     uint64
		Speed        uint32
		Manufacturer string
		PartNumber   string
		SerialNumber string
	}
	var memories []PhysicalMemory
	if err := wmi.Query("SELECT Capacity, Speed, Manufacturer, PartNumber, SerialNumber FROM Win32_PhysicalMemory", &memories); err == nil {
		for i, mem := range memories {
			data.WriteString(fmt.Sprintf("  Memory Module %d:\n", i+1))
			if mem.Capacity > 0 {
				data.WriteString(fmt.Sprintf("    Size: %d bytes\n", mem.Capacity))
			}
			if mem.Speed > 0 {
				data.WriteString(fmt.Sprintf("    Speed: %d MHz\n", mem.Speed))
			}
			if mem.Manufacturer != "" {
				data.WriteString(fmt.Sprintf("    Manufacturer: %s\n", mem.Manufacturer))
			}
			if mem.PartNumber != "" {
				data.WriteString(fmt.Sprintf("    Part Number: %s\n", mem.PartNumber))
			}
			if mem.SerialNumber != "" {
				data.WriteString(fmt.Sprintf("    Serial: %s\n", mem.SerialNumber))
			}
		}
	} else {
		logging.LogDebug(moduleName, fmt.Sprintf("Error querying Win32_PhysicalMemory: %v", err))
	}

	return data.String()
}
