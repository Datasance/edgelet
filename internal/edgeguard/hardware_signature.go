package edgeguard

import (
	"github.com/eclipse-iofog/agent-go/internal/hardware"
)

// DeleteHardwareSignature deletes the hardware signature file (exported for backward compatibility)
func DeleteHardwareSignature() error {
	return hardware.DeleteHardwareSignature()
}

// readHardwareSignature reads the hardware signature from the persistent file
func readHardwareSignature() (string, error) {
	return hardware.ReadHardwareSignature()
}

// writeHardwareSignature writes the hardware signature to the persistent file
func writeHardwareSignature(signature string) error {
	return hardware.WriteHardwareSignature(signature)
}

// getHardwareSignatureFilePath returns the path to the hardware signature file
func getHardwareSignatureFilePath() (string, error) {
	return hardware.GetHardwareSignatureFilePath()
}
