//go:build linux
// +build linux

package edgeguard

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/net"
)

func collectPlatformFingerprint(_ context.Context) FingerprintPayload {
	payload := FingerprintPayload{SchemaVersion: 1}
	dmiBase := "/sys/class/dmi/id"

	payload.System = SystemIdentity{
		UUID:         readDMIFile(dmiBase, "product_uuid"),
		Serial:       readDMIFile(dmiBase, "product_serial"),
		Manufacturer: readDMIFile(dmiBase, "sys_vendor"),
		Model:        readDMIFile(dmiBase, "product_name"),
	}
	payload.BIOS = BIOSIdentity{
		Manufacturer: readDMIFile(dmiBase, "bios_vendor"),
		Version:      readDMIFile(dmiBase, "bios_version"),
	}
	payload.Motherboard = MotherboardIdentity{
		Product: readDMIFile(dmiBase, "board_name"),
		Serial:  readDMIFile(dmiBase, "board_serial"),
	}

	payload.PCIDevices = collectLinuxPCIDevices()
	payload.GPUDevices = deriveGPUDevices(payload.PCIDevices)
	payload.RootDisk = collectLinuxRootDiskIdentity()
	payload.PrimaryNICs = collectLinuxPrimaryNICs()
	payload.USBDevices = collectLinuxUSBDevices()
	payload.PlatformDevices = collectLinuxPlatformDevices()

	payload.Optional = OptionalFingerprintSignals{
		MemoryModules: collectLinuxMemoryModules(),
		Firmware: FirmwareIdentity{
			SecureBootState: readLinuxSecureBootState(),
		},
		TPM:          collectLinuxTPMIdentity(),
		DMIBoardUUID: readDMIFile(dmiBase, "board_serial"),
		MachineID:    strings.TrimSpace(readSysFile("/etc/machine-id")),
	}

	return payload
}

func collectLinuxPCIDevices() []PCIDeviceIdentity {
	entries, err := os.ReadDir("/sys/bus/pci/devices")
	if err != nil {
		return nil
	}
	devices := make([]PCIDeviceIdentity, 0, len(entries))
	for _, entry := range entries {
		base := filepath.Join("/sys/bus/pci/devices", entry.Name())
		dev := PCIDeviceIdentity{
			Slot:            entry.Name(),
			Class:           readSysFile(filepath.Join(base, "class")),
			VendorID:        readSysFile(filepath.Join(base, "vendor")),
			DeviceID:        readSysFile(filepath.Join(base, "device")),
			SubsystemVendor: readSysFile(filepath.Join(base, "subsystem_vendor")),
			SubsystemDevice: readSysFile(filepath.Join(base, "subsystem_device")),
		}
		if dev.Class == "" && dev.VendorID == "" && dev.DeviceID == "" {
			continue
		}
		devices = append(devices, dev)
	}
	return devices
}

func collectLinuxRootDiskIdentity() DiskIdentity {
	identity := DiskIdentity{}
	partitions, err := disk.Partitions(false)
	if err != nil {
		return identity
	}
	rootDevice := ""
	for _, p := range partitions {
		if p.Mountpoint == "/" {
			rootDevice = p.Device
			break
		}
	}
	if rootDevice == "" {
		return identity
	}

	deviceName := filepath.Base(rootDevice)
	deviceName = strings.TrimSuffix(deviceName, filepath.Ext(deviceName))
	deviceName = strings.TrimRight(deviceName, "0123456789")
	devicePath := filepath.Join("/sys/block", deviceName)

	identity.DeviceID = rootDevice
	identity.Serial = readSysFile(filepath.Join(devicePath, "device/serial"))
	identity.WWN = readSysFile(filepath.Join(devicePath, "wwid"))
	identity.Model = readSysFile(filepath.Join(devicePath, "device/model"))

	return identity
}

func collectLinuxPrimaryNICs() []NICIdentity {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	nics := make([]NICIdentity, 0, len(interfaces))
	for _, iface := range interfaces {
		if !isPhysicalNetworkInterfaceLinux(iface.Name) {
			continue
		}

		mac := iface.HardwareAddr
		if mac == "" || mac == "00:00:00:00:00:00" || mac == "00:00:00:00:00:00:00:00" {
			continue
		}
		linkState := readSysFile(filepath.Join("/sys/class/net", iface.Name, "carrier"))
		if linkState == "" {
			linkState = "unknown"
		}
		nics = append(nics, NICIdentity{
			Name:      iface.Name,
			MAC:       mac,
			LinkState: linkState,
		})
	}
	return nics
}

func collectLinuxUSBDevices() []USBDeviceIdentity {
	entries, err := os.ReadDir("/sys/bus/usb/devices")
	if err != nil {
		return nil
	}
	devices := make([]USBDeviceIdentity, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, "usb") {
			continue
		}
		devicePath := filepath.Join("/sys/bus/usb/devices", name)
		vendorID := readSysFile(filepath.Join(devicePath, "idVendor"))
		productID := readSysFile(filepath.Join(devicePath, "idProduct"))
		if vendorID == "" || productID == "" {
			continue
		}
		class := readSysFile(filepath.Join(devicePath, "bDeviceClass"))
		if class == "09" {
			continue
		}
		devices = append(devices, USBDeviceIdentity{
			BusPath:      name,
			VendorID:     vendorID,
			ProductID:    productID,
			Manufacturer: readSysFile(filepath.Join(devicePath, "manufacturer")),
			Product:      readSysFile(filepath.Join(devicePath, "product")),
			Serial:       readSysFile(filepath.Join(devicePath, "serial")),
		})
	}
	return devices
}

func collectLinuxPlatformDevices() []PlatformDeviceIdentity {
	if runtime.GOARCH != "arm64" && runtime.GOARCH != "arm" {
		return nil
	}
	devs := make([]PlatformDeviceIdentity, 0)
	if model := strings.TrimSpace(readSysFile("/proc/device-tree/model")); model != "" {
		devs = append(devs, PlatformDeviceIdentity{Type: "devicetree", Name: "model", Value: model})
	}
	if compatible := strings.TrimSpace(readSysFile("/proc/device-tree/compatible")); compatible != "" {
		devs = append(devs, PlatformDeviceIdentity{Type: "devicetree", Name: "compatible", Value: compatible})
	}
	if serial := strings.TrimSpace(readSysFile("/proc/device-tree/serial-number")); serial != "" {
		devs = append(devs, PlatformDeviceIdentity{Type: "devicetree", Name: "serial-number", Value: serial})
	}
	return devs
}

func collectLinuxMemoryModules() []MemoryModuleIdentity {
	entries, err := os.ReadDir("/sys/devices/system/edac/mc")
	if err != nil {
		return nil
	}
	modules := make([]MemoryModuleIdentity, 0)
	for _, mc := range entries {
		if !strings.HasPrefix(mc.Name(), "mc") {
			continue
		}
		mcDir := filepath.Join("/sys/devices/system/edac/mc", mc.Name())
		dimmEntries, err := os.ReadDir(mcDir)
		if err != nil {
			continue
		}
		for _, dimm := range dimmEntries {
			if !strings.HasPrefix(dimm.Name(), "dimm") {
				continue
			}
			dimmDir := filepath.Join(mcDir, dimm.Name())
			mod := MemoryModuleIdentity{
				Locator:    readSysFile(filepath.Join(dimmDir, "dimm_label")),
				Serial:     readSysFile(filepath.Join(dimmDir, "dimm_serial")),
				PartNumber: readSysFile(filepath.Join(dimmDir, "dimm_part_number")),
			}
			if mod.Locator == "" && mod.Serial == "" && mod.PartNumber == "" {
				continue
			}
			modules = append(modules, mod)
		}
	}
	return modules
}

func readLinuxSecureBootState() string {
	base := "/sys/firmware/efi/efivars"
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "SecureBoot-") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(base, entry.Name()))
		if err != nil || len(data) == 0 {
			continue
		}
		last := data[len(data)-1]
		if last == 1 {
			return "enabled"
		}
		return "disabled"
	}
	return ""
}

func collectLinuxTPMIdentity() TPMIdentity {
	tpm := TPMIdentity{}
	if _, err := os.Stat("/sys/class/tpm/tpm0"); err == nil {
		tpm.Present = true
	}

	ekCertPath := "/sys/class/tpm/tpm0/device/ekcert"
	if data, err := os.ReadFile(ekCertPath); err == nil && len(data) > 0 {
		sum := sha256.Sum256(data)
		tpm.EKFingerprint = base64.StdEncoding.EncodeToString(sum[:])
	}

	if data, err := os.ReadFile("/sys/class/tpm/tpm0/pcrs"); err == nil && len(data) > 0 {
		sum := sha256.Sum256(data)
		tpm.PCRPolicyHash = base64.StdEncoding.EncodeToString(sum[:])
	}
	return tpm
}

func readSysFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readDMIFile(base, name string) string {
	value := strings.TrimSpace(readSysFile(filepath.Join(base, name)))
	if value == "" {
		return ""
	}

	normalized := strings.ToLower(value)
	switch normalized {
	case "none", "not specified", "not available", "unknown", "to be filled by o.e.m.":
		return ""
	default:
		return value
	}
}

func isPhysicalNetworkInterfaceLinux(name string) bool {
	if name == "" {
		return false
	}

	virtualPrefixes := []string{
		"lo", "docker", "br-", "veth", "virbr", "tun", "tap", "vmnet", "vnet", "cni", "flannel", "cali",
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}

	physicalPrefixes := []string{
		"eth", "en", "em", "eno", "ens", "enp", "wlan", "wlp", "wwan",
	}
	for _, prefix := range physicalPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}

	return false
}
