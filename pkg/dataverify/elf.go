package dataverify

import (
	"fmt"
	"os"
)

const elfMagic = "\x7fELF"

// FatRuntimeName is the fat daemon ELF inside the extracted bundle bin/ directory.
const FatRuntimeName = "edgelet"

// IsELF reports whether path points to a file with a valid ELF header.
func IsELF(path string) (bool, error) {
	f, err := os.Open(path) // #nosec G304 -- path under verified extract root from caller
	if err != nil {
		return false, err
	}
	defer func() {
		_ = f.Close()
	}()

	hdr := make([]byte, 4)
	n, err := f.Read(hdr)
	if err != nil {
		return false, err
	}
	if n < len(elfMagic) {
		return false, nil
	}
	return string(hdr) == elfMagic, nil
}

// VerifyFatRuntime checks that the fat edgelet runtime exists and is an ELF.
func VerifyFatRuntime(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", FatRuntimeName, err)
	}
	if info.IsDir() {
		return fmt.Errorf("%s is a directory", FatRuntimeName)
	}
	ok, err := IsELF(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", FatRuntimeName, err)
	}
	if !ok {
		return fmt.Errorf("%s is not an ELF binary", FatRuntimeName)
	}
	return nil
}
