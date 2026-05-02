//go:build windows
// +build windows

package edgeguard

import (
	"context"
	"strings"

	"github.com/yusufpapurcu/wmi"
)

func collectPlatformFingerprint(_ context.Context) FingerprintPayload {
	payload := FingerprintPayload{SchemaVersion: 1}
	payload.System, payload.BIOS, payload.Motherboard = collectWindowsSystemBIOSBoard()
	payload.PCIDevices = collectWindowsPCIDevices()
	payload.GPUDevices = deriveGPUDevices(payload.PCIDevices)
	payload.RootDisk = collectWindowsRootDiskIdentity()
	payload.PrimaryNICs = collectWindowsPrimaryNICs()
	payload.USBDevices = collectWindowsUSBDevices()
	payload.Optional = OptionalFingerprintSignals{
		MemoryModules: collectWindowsMemoryModules(),
		TPM:           collectWindowsTPMIdentity(),
	}
	return payload
}

func collectWindowsSystemBIOSBoard() (SystemIdentity, BIOSIdentity, MotherboardIdentity) {
	sys := SystemIdentity{}
	bios := BIOSIdentity{}
	board := MotherboardIdentity{}

	type computerSystem struct {
		Manufacturer string
		Model        string
	}
	var systems []computerSystem
	_ = wmi.Query("SELECT Manufacturer, Model FROM Win32_ComputerSystem", &systems)
	if len(systems) > 0 {
		sys.Manufacturer = systems[0].Manufacturer
		sys.Model = systems[0].Model
	}

	type computerSystemProduct struct {
		UUID string
	}
	var products []computerSystemProduct
	_ = wmi.Query("SELECT UUID FROM Win32_ComputerSystemProduct", &products)
	if len(products) > 0 {
		sys.UUID = products[0].UUID
	}

	type baseBoard struct {
		Product      string
		SerialNumber string
	}
	var boards []baseBoard
	_ = wmi.Query("SELECT Product, SerialNumber FROM Win32_BaseBoard", &boards)
	if len(boards) > 0 {
		board.Product = boards[0].Product
		board.Serial = boards[0].SerialNumber
	}

	type biosRow struct {
		Manufacturer string
		Version      string
	}
	var biosRows []biosRow
	_ = wmi.Query("SELECT Manufacturer, Version FROM Win32_BIOS", &biosRows)
	if len(biosRows) > 0 {
		bios.Manufacturer = biosRows[0].Manufacturer
		bios.Version = biosRows[0].Version
	}

	return sys, bios, board
}

func collectWindowsPCIDevices() []PCIDeviceIdentity {
	type pnp struct {
		DeviceID string
		PNPClass string
	}
	var rows []pnp
	_ = wmi.Query("SELECT DeviceID, PNPClass FROM Win32_PnPEntity WHERE DeviceID LIKE 'PCI%'", &rows)
	devices := make([]PCIDeviceIdentity, 0, len(rows))
	for _, r := range rows {
		dev := PCIDeviceIdentity{
			Slot:  r.DeviceID,
			Class: r.PNPClass,
		}
		parts := strings.Split(r.DeviceID, "\\")
		if len(parts) > 1 {
			idPart := parts[1]
			segments := strings.Split(idPart, "&")
			for _, seg := range segments {
				if strings.HasPrefix(seg, "VEN_") {
					dev.VendorID = strings.TrimPrefix(seg, "VEN_")
				}
				if strings.HasPrefix(seg, "DEV_") {
					dev.DeviceID = strings.TrimPrefix(seg, "DEV_")
				}
				if strings.HasPrefix(seg, "SUBSYS_") {
					sub := strings.TrimPrefix(seg, "SUBSYS_")
					if len(sub) >= 8 {
						dev.SubsystemDevice = sub[:4]
						dev.SubsystemVendor = sub[4:]
					}
				}
			}
		}
		devices = append(devices, dev)
	}
	return devices
}

func collectWindowsRootDiskIdentity() DiskIdentity {
	type diskDrive struct {
		DeviceID     string
		SerialNumber string
		Model        string
	}
	var drives []diskDrive
	_ = wmi.Query("SELECT DeviceID, SerialNumber, Model FROM Win32_DiskDrive", &drives)
	if len(drives) == 0 {
		return DiskIdentity{}
	}
	return DiskIdentity{
		DeviceID: drives[0].DeviceID,
		Serial:   drives[0].SerialNumber,
		Model:    drives[0].Model,
	}
}

func collectWindowsPrimaryNICs() []NICIdentity {
	type adapter struct {
		Name            string
		MACAddress      string
		PhysicalAdapter bool
		NetConnectionID string
		Status          string
	}
	var adapters []adapter
	_ = wmi.Query("SELECT Name, MACAddress, PhysicalAdapter, NetConnectionID, Status FROM Win32_NetworkAdapter", &adapters)
	nics := make([]NICIdentity, 0, len(adapters))
	for _, a := range adapters {
		if !a.PhysicalAdapter || a.MACAddress == "" {
			continue
		}
		linkState := "0"
		if strings.Contains(strings.ToLower(a.Status), "up") || strings.Contains(strings.ToLower(a.Status), "connected") {
			linkState = "1"
		}
		name := a.NetConnectionID
		if name == "" {
			name = a.Name
		}
		nics = append(nics, NICIdentity{Name: name, MAC: a.MACAddress, LinkState: linkState})
	}
	return nics
}

func collectWindowsUSBDevices() []USBDeviceIdentity {
	type pnp struct {
		Name         string
		DeviceID     string
		Manufacturer string
	}
	var rows []pnp
	_ = wmi.Query("SELECT Name, DeviceID, Manufacturer FROM Win32_PnPEntity WHERE DeviceID LIKE 'USB%'", &rows)
	devices := make([]USBDeviceIdentity, 0, len(rows))
	for _, r := range rows {
		dev := USBDeviceIdentity{
			BusPath:      r.DeviceID,
			Manufacturer: r.Manufacturer,
			Product:      r.Name,
		}
		parts := strings.Split(r.DeviceID, "\\")
		if len(parts) > 1 {
			segments := strings.Split(parts[1], "&")
			for _, seg := range segments {
				if strings.HasPrefix(seg, "VID_") {
					dev.VendorID = strings.TrimPrefix(seg, "VID_")
				}
				if strings.HasPrefix(seg, "PID_") {
					dev.ProductID = strings.TrimPrefix(seg, "PID_")
				}
			}
		}
		devices = append(devices, dev)
	}
	return devices
}

func collectWindowsMemoryModules() []MemoryModuleIdentity {
	type memory struct {
		BankLabel    string
		SerialNumber string
		PartNumber   string
	}
	var rows []memory
	_ = wmi.Query("SELECT BankLabel, SerialNumber, PartNumber FROM Win32_PhysicalMemory", &rows)
	modules := make([]MemoryModuleIdentity, 0, len(rows))
	for _, r := range rows {
		modules = append(modules, MemoryModuleIdentity{
			Locator:    r.BankLabel,
			Serial:     r.SerialNumber,
			PartNumber: strings.TrimSpace(r.PartNumber),
		})
	}
	return modules
}

func collectWindowsTPMIdentity() TPMIdentity {
	type tpmInfo struct {
		SpecVersion string
	}
	var rows []tpmInfo
	err := wmi.Query("SELECT SpecVersion FROM Win32_Tpm", &rows)
	if err != nil || len(rows) == 0 {
		return TPMIdentity{}
	}
	return TPMIdentity{Present: true}
}
