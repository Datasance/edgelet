package dataverify

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Verify checks sha256sums and symlinks from manifest files in dir, then
// validates the fat edgelet runtime ELF in dir.
func Verify(dir string) error {
	if err := VerifySums(dir, ".sha256sums"); err != nil {
		return fmt.Errorf("verify sums: %w", err)
	}
	if err := VerifyLinks(dir, ".links"); err != nil {
		return fmt.Errorf("verify links: %w", err)
	}
	if err := VerifyFatRuntime(filepath.Join(dir, FatRuntimeName)); err != nil {
		return fmt.Errorf("verify fat runtime: %w", err)
	}
	if err := VerifyNetAux(dir); err != nil {
		return fmt.Errorf("verify net aux: %w", err)
	}
	return nil
}

// VerifyNetAux checks userland net tools staged into the embed bundle bin/ tree.
func VerifyNetAux(binDir string) error {
	required := []string{
		filepath.Join("aux", "xtables-legacy-multi"),
		"ip",
		"busybox",
	}
	for _, rel := range required {
		path := filepath.Join(binDir, rel)
		if st, err := os.Stat(path); err != nil {
			return fmt.Errorf("missing %s: %w", rel, err)
		} else if st.IsDir() {
			return fmt.Errorf("%s is a directory", rel)
		}
	}
	legacy := filepath.Join(binDir, "aux", "iptables")
	target, err := os.Readlink(legacy)
	if err != nil {
		return fmt.Errorf("aux/iptables symlink: %w", err)
	}
	if target != "xtables-legacy-multi" {
		return fmt.Errorf("aux/iptables -> %q, want xtables-legacy-multi", target)
	}
	return nil
}

// VerifySums verifies file hashes listed in sumListFile relative to root.
func VerifySums(root, sumListFile string) error {
	sums, err := fileMapFields(filepath.Join(root, sumListFile), 1, 0)
	if err != nil {
		return err
	}
	if len(sums) == 0 {
		return fmt.Errorf("no entries found in %s", sumListFile)
	}
	numFailed := 0
	for sumFile, sumExpected := range sums {
		file := filepath.Join(root, sumFile)
		sumActual, err := sha256Sum(file)
		if err != nil {
			return err
		}
		if sumExpected != sumActual {
			numFailed++
		}
	}
	if numFailed != 0 {
		return fmt.Errorf("failed %d hash verifications", numFailed)
	}
	return nil
}

// VerifyLinks verifies symlinks listed in linkListFile relative to root.
func VerifyLinks(root, linkListFile string) error {
	links, err := fileMapFields(filepath.Join(root, linkListFile), 0, 1)
	if err != nil {
		return err
	}
	if len(links) == 0 {
		return fmt.Errorf("no entries found in %s", linkListFile)
	}
	numFailed := 0
	for linkFile, linkExpected := range links {
		file := filepath.Join(root, linkFile)
		linkActual, err := os.Readlink(file)
		if err != nil {
			return err
		}
		if linkExpected != linkActual {
			numFailed++
		}
	}
	if numFailed != 0 {
		return fmt.Errorf("failed %d link verifications", numFailed)
	}
	return nil
}

func fileMapFields(fileName string, key, val int) (map[string]string, error) {
	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if len(fields) <= key || len(fields) <= val {
			return nil, fmt.Errorf("fields for file %s (%d) smaller than required index (key: %d, val: %d)", fileName, len(fields), key, val)
		}
		result[fields[key]] = fields[val]
	}
	return result, scanner.Err()
}

func sha256Sum(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
