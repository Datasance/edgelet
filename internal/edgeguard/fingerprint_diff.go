package edgeguard

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/eclipse-iofog/edgelet/internal/utils/logging"
)

func logFingerprintDiff(previous, current FingerprintPayload) {
	previousCanonical, errPrev := canonicalizeFingerprintPayload(previous)
	currentCanonical, errCur := canonicalizeFingerprintPayload(current)
	if errPrev != nil || errCur != nil || previousCanonical == currentCanonical {
		return
	}

	changed := make([]string, 0)
	if !reflect.DeepEqual(previous.System, current.System) {
		changed = append(changed, "system")
	}
	if !reflect.DeepEqual(previous.BIOS, current.BIOS) {
		changed = append(changed, "bios")
	}
	if !reflect.DeepEqual(previous.Motherboard, current.Motherboard) {
		changed = append(changed, "motherboard")
	}
	if !reflect.DeepEqual(previous.CPU, current.CPU) {
		changed = append(changed, "cpu")
	}
	if !reflect.DeepEqual(previous.PCIDevices, current.PCIDevices) {
		changed = append(changed, "pciDevices")
	}
	if !reflect.DeepEqual(previous.RootDisk, current.RootDisk) {
		changed = append(changed, "rootDisk")
	}
	if !reflect.DeepEqual(previous.PrimaryNICs, current.PrimaryNICs) {
		changed = append(changed, "primaryNICs")
	}
	if !reflect.DeepEqual(previous.USBDevices, current.USBDevices) {
		changed = append(changed, "usbDevices")
	}
	if !reflect.DeepEqual(previous.PlatformDevices, current.PlatformDevices) {
		changed = append(changed, "platformDevices")
	}
	if !reflect.DeepEqual(previous.Optional, current.Optional) {
		changed = append(changed, "optional")
	}

	if len(changed) > 0 {
		logging.LogWarn(moduleName, fmt.Sprintf("Fingerprint diff detected in fields: %s", strings.Join(changed, ", ")))
	}
}
