//go:build darwin
// +build darwin

package edgeguard

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

func collectPlatformFingerprint(_ context.Context) FingerprintPayload {
	payload := FingerprintPayload{SchemaVersion: 1}
	payload.System, payload.BIOS, payload.Motherboard = collectDarwinSystemBIOSBoard()
	payload.PCIDevices = collectDarwinPCIDevices()
	payload.GPUDevices = deriveGPUDevices(payload.PCIDevices)
	payload.RootDisk = collectDarwinRootDiskIdentity()
	payload.PrimaryNICs = collectDarwinPrimaryNICs()
	payload.USBDevices = collectDarwinUSBDevices()
	payload.Optional = OptionalFingerprintSignals{
		MemoryModules: collectDarwinMemoryModules(),
	}
	return payload
}

func collectDarwinSystemBIOSBoard() (SystemIdentity, BIOSIdentity, MotherboardIdentity) {
	sys := SystemIdentity{}
	bios := BIOSIdentity{}
	board := MotherboardIdentity{}

	cmd := exec.Command("ioreg", "-c", "IOPlatformExpertDevice", "-d", "2")
	out, err := cmd.Output()
	if err == nil {
		output := string(out)
		sys.UUID = extractIORegValue(output, "IOPlatformUUID")
		sys.Serial = extractIORegValue(output, "IOPlatformSerialNumber")
		sys.Model = extractIORegValue(output, "model")
		sys.Manufacturer = extractIORegValue(output, "manufacturer")
		board.Product = extractIORegValue(output, "board-id")
	}

	cmd = exec.Command("system_profiler", "SPHardwareDataType")
	out, err = cmd.Output()
	if err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			line = strings.TrimSpace(line)
			switch {
			case strings.HasPrefix(line, "Model Name:"):
				sys.Model = strings.TrimSpace(strings.TrimPrefix(line, "Model Name:"))
			case strings.HasPrefix(line, "Hardware UUID:"):
				sys.UUID = strings.TrimSpace(strings.TrimPrefix(line, "Hardware UUID:"))
			case strings.HasPrefix(line, "Serial Number"):
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					sys.Serial = strings.TrimSpace(parts[1])
				}
			case strings.HasPrefix(line, "Boot ROM Version:"):
				bios.Version = strings.TrimSpace(strings.TrimPrefix(line, "Boot ROM Version:"))
				bios.Manufacturer = "apple"
			}
		}
	}
	return sys, bios, board
}

func collectDarwinPCIDevices() []PCIDeviceIdentity {
	cmd := exec.Command("system_profiler", "SPPCIDataType")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	devices := make([]PCIDeviceIdentity, 0)
	current := PCIDeviceIdentity{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasSuffix(line, ":") && !strings.Contains(line, "ID") {
			if current.VendorID != "" || current.DeviceID != "" || current.Class != "" {
				devices = append(devices, current)
			}
			current = PCIDeviceIdentity{Slot: strings.TrimSuffix(line, ":")}
			continue
		}
		if strings.HasPrefix(line, "Vendor ID:") {
			current.VendorID = strings.TrimSpace(strings.TrimPrefix(line, "Vendor ID:"))
		}
		if strings.HasPrefix(line, "Device ID:") {
			current.DeviceID = strings.TrimSpace(strings.TrimPrefix(line, "Device ID:"))
		}
		if strings.HasPrefix(line, "Revision ID:") {
			current.Class = strings.TrimSpace(strings.TrimPrefix(line, "Revision ID:"))
		}
	}
	if current.VendorID != "" || current.DeviceID != "" || current.Class != "" {
		devices = append(devices, current)
	}
	return devices
}

func collectDarwinRootDiskIdentity() DiskIdentity {
	identity := DiskIdentity{}
	partitions, err := disk.Partitions(false)
	if err != nil {
		return identity
	}
	root := ""
	for _, p := range partitions {
		if p.Mountpoint == "/" {
			root = p.Device
			break
		}
	}
	if root == "" {
		return identity
	}
	identity.DeviceID = root

	cmd := exec.Command("diskutil", "info", root)
	out, err := cmd.Output()
	if err != nil {
		return identity
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "Device / Media Name:"):
			identity.Model = strings.TrimSpace(strings.TrimPrefix(line, "Device / Media Name:"))
		case strings.HasPrefix(line, "Disk / Partition UUID:"):
			identity.WWN = strings.TrimSpace(strings.TrimPrefix(line, "Disk / Partition UUID:"))
		}
	}
	return identity
}

func collectDarwinPrimaryNICs() []NICIdentity {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	nics := make([]NICIdentity, 0, len(interfaces))
	for _, iface := range interfaces {
		if !isPhysicalNetworkInterfaceDarwin(iface.Name) {
			continue
		}
		mac := iface.HardwareAddr
		if mac == "" || mac == "00:00:00:00:00:00" {
			continue
		}
		linkState := "unknown"
		cmd := exec.Command("ifconfig", iface.Name) // #nosec G204 -- iface names come from OS interface list
		if out, err := cmd.Output(); err == nil {
			if strings.Contains(string(out), "status: active") {
				linkState = "1"
			} else if strings.Contains(string(out), "status: inactive") {
				linkState = "0"
			}
		}
		nics = append(nics, NICIdentity{Name: iface.Name, MAC: mac, LinkState: linkState})
	}
	return nics
}

func collectDarwinUSBDevices() []USBDeviceIdentity {
	cmd := exec.Command("system_profiler", "SPUSBDataType")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	lines := strings.Split(string(out), "\n")
	devices := make([]USBDeviceIdentity, 0)
	current := USBDeviceIdentity{}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, "USB") {
			if current.VendorID != "" || current.ProductID != "" {
				devices = append(devices, current)
			}
			current = USBDeviceIdentity{BusPath: strings.TrimSuffix(trimmed, ":")}
			continue
		}
		if strings.HasPrefix(trimmed, "Vendor ID:") {
			current.VendorID = parseDarwinHexID(trimmed)
		}
		if strings.HasPrefix(trimmed, "Product ID:") {
			current.ProductID = parseDarwinHexID(trimmed)
		}
		if strings.HasPrefix(trimmed, "Manufacturer:") {
			current.Manufacturer = strings.TrimSpace(strings.TrimPrefix(trimmed, "Manufacturer:"))
		}
		if strings.HasPrefix(trimmed, "Product ID:") && current.Product == "" {
			current.Product = strings.TrimSpace(strings.TrimPrefix(trimmed, "Product ID:"))
		}
		if strings.HasPrefix(trimmed, "Serial Number:") {
			current.Serial = strings.TrimSpace(strings.TrimPrefix(trimmed, "Serial Number:"))
		}
	}
	if current.VendorID != "" || current.ProductID != "" {
		devices = append(devices, current)
	}
	return devices
}

func parseDarwinHexID(line string) string {
	re := regexp.MustCompile(`0x([0-9a-fA-F]+)`)
	m := re.FindStringSubmatch(line)
	if len(m) > 1 {
		return m[1]
	}
	return strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
}

func extractIORegValue(output, key string) string {
	patterns := []string{
		`"` + regexp.QuoteMeta(key) + `"\s*=\s*"([^"]+)"`,
		`"` + regexp.QuoteMeta(key) + `"\s*=\s*<"([^"]+)">`,
	}
	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		m := re.FindStringSubmatch(output)
		if len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func isPhysicalNetworkInterfaceDarwin(name string) bool {
	if name == "" {
		return false
	}
	virtualPrefixes := []string{
		"lo", "awdl", "llw", "utun", "anpi", "bridge", "gif", "stf", "vmnet", "vnic",
		"docker", "br-", "veth", "tun", "tap",
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	physicalPrefixes := []string{
		"en", "eth", "wlan", "wifi",
	}
	for _, prefix := range physicalPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func collectDarwinMemoryModules() []MemoryModuleIdentity {
	cmd := exec.Command("system_profiler", "SPMemoryDataType")
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	modules := make([]MemoryModuleIdentity, 0)
	current := MemoryModuleIdentity{}
	for _, line := range strings.Split(string(out), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "BANK") || strings.HasPrefix(trimmed, "DIMM") {
			if current.Locator != "" || current.Serial != "" || current.PartNumber != "" {
				modules = append(modules, current)
			}
			current = MemoryModuleIdentity{Locator: trimmed}
			continue
		}
		if strings.HasPrefix(trimmed, "Serial Number:") {
			current.Serial = strings.TrimSpace(strings.TrimPrefix(trimmed, "Serial Number:"))
		}
		if strings.HasPrefix(trimmed, "Part Number:") {
			current.PartNumber = strings.TrimSpace(strings.TrimPrefix(trimmed, "Part Number:"))
		}
	}
	if current.Locator != "" || current.Serial != "" || current.PartNumber != "" {
		modules = append(modules, current)
	}
	return modules
}
