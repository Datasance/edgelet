package edgeguard

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/eclipse-iofog/agent-go/internal/config"
	"github.com/eclipse-iofog/agent-go/internal/fieldagent"
	"github.com/eclipse-iofog/agent-go/internal/hardware"
	"github.com/eclipse-iofog/agent-go/internal/models"
	"github.com/eclipse-iofog/agent-go/internal/statusreporter"
	"github.com/eclipse-iofog/agent-go/internal/utils/logging"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
)

const (
	moduleName                = "Edge Guard Manager"
	hardwareCollectionTimeout = 3 * time.Second
)

// JWK represents a JSON Web Key structure for Ed25519
type JWK struct {
	Kty string `json:"kty"` // Key type (must be "OKP")
	Crv string `json:"crv"` // Curve (must be "Ed25519")
	D   string `json:"d"`   // Private key (base64url encoded)
	X   string `json:"x"`   // Public key (base64url encoded)
	Kid string `json:"kid"` // Key ID (optional)
}

// Manager manages edge device security and attestation
type Manager struct {
	config            *config.Config
	ctx               context.Context
	cancel            context.CancelFunc
	attestationTicker *time.Ticker
	privateKey        ed25519.PrivateKey
	previousFrequency int64
	mu                sync.Mutex
}

var (
	instance *Manager
	once     sync.Once
)

// GetInstance returns the singleton Edge Guard Manager instance
func GetInstance() *Manager {
	once.Do(func() {
		instance = &Manager{
			config: config.GetInstance(),
		}
		instance.ctx, instance.cancel = context.WithCancel(context.Background())
	})
	return instance
}

// Start starts the Edge Guard Manager
func (m *Manager) Start() error {
	logging.LogInfo(moduleName, "Starting Edge Guard Manager")

	// Check if agent is provisioned before starting Edge Guard
	// Hardware signature requires private key, which only exists when agent is provisioned
	if m.config.IOFogUUID == "" || m.config.PrivateKey == "" {
		logging.LogInfo(moduleName, "Edge Guard Manager skipped - agent not provisioned (no private key)")
		return nil
	}

	// Check Edge Guard frequency - if 0, Edge Guard is disabled
	attestationFreq := m.config.EdgeGuardFrequency
	if attestationFreq <= 0 {
		// Edge Guard disabled - delete .jwt file if exists
		if err := hardware.DeleteHardwareSignature(); err != nil {
			logging.LogDebug(moduleName, fmt.Sprintf("No hardware signature file to delete (expected if file doesn't exist): %v", err))
		} else {
			logging.LogInfo(moduleName, "Edge Guard disabled - hardware signature file deleted")
		}
		logging.LogInfo(moduleName, "Edge Guard Manager disabled (frequency = 0)")
		return nil
	}

	// Edge Guard enabled (frequency > 0)
	// On initial startup, preserve existing .jwt file if it exists (Edge Guard was active before reboot)
	// If file doesn't exist, checkHardwareSignature() will create it
	// Check hardware signature on start
	m.checkHardwareSignature()

	// Start periodic attestation
	duration := time.Duration(attestationFreq) * time.Second
	m.attestationTicker = time.NewTicker(duration)
	go m.attestationWorker()
	m.previousFrequency = attestationFreq
	logging.LogInfo(moduleName, fmt.Sprintf("Edge Guard Manager started with attestation frequency: %d seconds", attestationFreq))

	return nil
}

// Stop stops the Edge Guard Manager
func (m *Manager) Stop() error {
	logging.LogInfo(moduleName, "Stopping Edge Guard Manager")
	if m.cancel != nil {
		m.cancel()
	}
	if m.attestationTicker != nil {
		m.attestationTicker.Stop()
	}
	return nil
}

// attestationWorker performs periodic attestation
func (m *Manager) attestationWorker() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-m.attestationTicker.C:
			m.performAttestation()
		}
	}
}

// checkHardwareSignature checks hardware signature
func (m *Manager) checkHardwareSignature() {
	logging.LogDebug(moduleName, "Started checking hardware signature")

	// Verify agent is provisioned before attempting to sign
	if m.config.IOFogUUID == "" || m.config.PrivateKey == "" {
		logging.LogDebug(moduleName, "Skipping hardware signature check - agent not provisioned")
		return
	}

	// Check if .jwt file exists before collecting signature
	filePath, err := getHardwareSignatureFilePath()
	var fileExists bool
	if err == nil {
		if _, err := os.Stat(filePath); err == nil {
			fileExists = true
			logging.LogDebug(moduleName, fmt.Sprintf("Hardware signature file exists: %s (will compare)", filePath))
		} else {
			fileExists = false
			logging.LogDebug(moduleName, fmt.Sprintf("Hardware signature file does not exist: %s (will create)", filePath))
		}
	}

	newSignature, err := m.collectAndSignHardwareSignature()
	if err != nil {
		logging.LogError(moduleName, "Failed to collect hardware signature", err)
		return
	}

	// Read stored signature from separate file (not config.yaml to avoid triggering SIGHUP)
	storedSignature, err := readHardwareSignature()
	if err != nil {
		logging.LogError(moduleName, "Failed to read stored hardware signature", err)
		// Continue with empty signature (will treat as first run)
		storedSignature = ""
	}

	if storedSignature == "" || !fileExists {
		// First run - store signature to separate file
		logging.LogInfo(moduleName, "Storing initial hardware signature (file does not exist or is empty)")
		if err := writeHardwareSignature(newSignature); err != nil {
			logging.LogError(moduleName, "Failed to write hardware signature", err)
			return
		}
		// Verify file was created
		if filePath, err := getHardwareSignatureFilePath(); err == nil {
			if _, err := os.Stat(filePath); err == nil {
				logging.LogInfo(moduleName, fmt.Sprintf("Hardware signature file successfully created and verified: %s", filePath))
			} else {
				logging.LogWarn(moduleName, fmt.Sprintf("Hardware signature write succeeded but file not found: %s", filePath))
			}
		}
		return
	}

	// Compare signatures
	if newSignature != storedSignature {
		logging.LogWarn(moduleName, "Hardware signature mismatch detected")
		logging.LogDebug(moduleName, fmt.Sprintf("Stored signature (first 50 chars): %s...", storedSignature[:min(50, len(storedSignature))]))
		logging.LogDebug(moduleName, fmt.Sprintf("New signature (first 50 chars): %s...", newSignature[:min(50, len(newSignature))]))
		m.handleHardwareChange()
	} else {
		logging.LogDebug(moduleName, "Hardware signature verification successful - signatures match")
		if fileExists {
			logging.LogDebug(moduleName, "Hardware signature file preserved and verified")
		}
	}
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// isPhysicalNetworkInterface determines if a network interface is physical (not virtual)
// Uses hybrid approach: whitelist for common physical patterns + blacklist for known virtual patterns
func (m *Manager) isPhysicalNetworkInterface(name string) bool {
	// Blacklist: exclude known virtual interface patterns
	virtualPatterns := []string{
		"docker", "br-", "veth", "lo", "lo0", "virbr", "vmnet", "tun", "tap",
		"bridge", "gif", "stf", "utun", "awdl", "llw", "anpi", "ap1",
	}
	for _, pattern := range virtualPatterns {
		if strings.HasPrefix(name, pattern) {
			return false
		}
	}

	// Whitelist: include known physical interface patterns
	physicalPatterns := []string{
		"eth", "en", "em", "wlan", "wlp", "wifi", "airport",
	}
	for _, pattern := range physicalPatterns {
		if strings.HasPrefix(name, pattern) {
			return true
		}
	}

	// For interfaces not matching whitelist patterns, exclude them to be safe
	// This ensures we only include interfaces we're confident are physical
	return false
}

// handleHardwareChange handles hardware change detection
// Matching Java: EdgeGuardManager.handleHardwareChange()
func (m *Manager) handleHardwareChange() {
	logging.LogWarn(moduleName, "Hardware change detected - deprovisioning agent")

	// Wrap in error handling to match Java's try-catch
	defer func() {
		if r := recover(); r != nil {
			logging.LogError(moduleName, "Error handling hardware change", fmt.Errorf("%v", r))
		}
	}()

	// 1. Update daemon status to WARNING with message "HW signature changed"
	// Matching Java: StatusReporter.setSupervisorStatus().setDaemonStatus(Constants.ModulesStatus.WARNING)
	//                StatusReporter.setSupervisorStatus().setWarningMessage("HW signature changed")
	statusreporter.GetInstance().UpdateSupervisorStatus(func(status *models.SupervisorStatus) {
		status.SetDaemonStatus(models.ModuleStatusWarning)
		status.SetWarningMessage("HW signature changed")
	})

	// 2. Immediately send status to controller
	// Matching Java: FieldAgent.getInstance().postStatusHelper()
	fieldagent.GetInstance().PostStatusHelper()

	// 3. Deprovision the agent (false = don't clear credentials, matching Java's deProvision(false))
	// Matching Java: FieldAgent.getInstance().deProvision(false)
	if err := fieldagent.GetInstance().Deprovision(false); err != nil {
		logging.LogError(moduleName, "Failed to deprovision agent", err)
	}

	// 4. Delete the signature file (not config.yaml to avoid triggering SIGHUP)
	if err := hardware.DeleteHardwareSignature(); err != nil {
		logging.LogError(moduleName, "Failed to delete hardware signature file", err)
	}
}

// collectAndSignHardwareSignature collects hardware info and signs it
func (m *Manager) collectAndSignHardwareSignature() (string, error) {
	logging.LogDebug(moduleName, "Starting hardware signature collection")

	hardwareData := m.collectHardwareInfo()
	logging.LogDebug(moduleName, fmt.Sprintf("Collected hardware data length: %d", len(hardwareData)))

	hash := m.hashData(hardwareData)
	signature, err := m.signWithPrivateKey(hash)
	if err != nil {
		return "", fmt.Errorf("failed to sign: %w", err)
	}

	return signature, nil
}

// collectHardwareInfo collects hardware information
func (m *Manager) collectHardwareInfo() string {
	var data strings.Builder

	// CPU info with cache information
	data.WriteString("=== CPU Information ===\n")
	if cpuInfo, err := cpu.Info(); err == nil && len(cpuInfo) > 0 {
		cpu := cpuInfo[0]
		data.WriteString(fmt.Sprintf("Architecture: %s\n", cpu.ModelName))
		data.WriteString(fmt.Sprintf("CPU(s): %d\n", cpu.Cores))
		data.WriteString(fmt.Sprintf("Model name: %s\n", cpu.ModelName))
		data.WriteString(fmt.Sprintf("CPU family: %s\n", cpu.Family))
		data.WriteString(fmt.Sprintf("Model: %s\n", cpu.Model))
		data.WriteString(fmt.Sprintf("Stepping: %d\n", cpu.Stepping))
		data.WriteString(fmt.Sprintf("Vendor ID: %s\n", cpu.VendorID))
		if cpu.CacheSize > 0 {
			data.WriteString(fmt.Sprintf("Cache Size: %d\n", cpu.CacheSize))
		}
		if len(cpu.Flags) > 0 {
			data.WriteString(fmt.Sprintf("Flags: %s\n", strings.Join(cpu.Flags, " ")))
		}
	} else {
		data.WriteString("CPU Information: Error collecting data\n")
	}

	// System/Motherboard/BIOS info (platform-specific)
	ctx, cancel := context.WithTimeout(context.Background(), hardwareCollectionTimeout)
	systemInfo := m.collectSystemInfoWithTimeout(ctx)
	cancel()
	data.WriteString(systemInfo)

	// Memory info
	data.WriteString("\n=== Memory Information ===\n")
	if memInfo, err := mem.VirtualMemory(); err == nil {
		data.WriteString(fmt.Sprintf("Memory:\n  Total: %d\n", memInfo.Total))
	} else {
		data.WriteString("Memory Information: Error collecting data\n")
	}

	// Physical memory modules (platform-specific)
	ctx, cancel = context.WithTimeout(context.Background(), hardwareCollectionTimeout)
	memoryInfo := m.collectMemoryInfoWithTimeout(ctx)
	cancel()
	data.WriteString(memoryInfo)

	// Storage info
	data.WriteString("\n=== Storage Information ===\n")
	if partitions, err := disk.Partitions(false); err == nil {
		for _, part := range partitions {
			if usage, err := disk.Usage(part.Mountpoint); err == nil {
				data.WriteString(fmt.Sprintf("Disk: %s Total=%d\n", part.Device, usage.Total))
			}
		}
	} else {
		data.WriteString("Storage Information: Error collecting data\n")
	}

	// Detailed storage info (platform-specific)
	ctx, cancel = context.WithTimeout(context.Background(), hardwareCollectionTimeout)
	storageInfo := m.collectStorageInfoWithTimeout(ctx)
	cancel()
	data.WriteString(storageInfo)

	// PCI devices (platform-specific)
	ctx, cancel = context.WithTimeout(context.Background(), hardwareCollectionTimeout)
	pciInfo := m.collectPciInfoWithTimeout(ctx)
	cancel()
	data.WriteString(pciInfo)

	// Network interfaces - filter to only include physical interfaces
	data.WriteString("\n=== Network Information ===\n")
	if interfaces, err := net.Interfaces(); err == nil {
		for _, iface := range interfaces {
			// Apply hybrid filtering: whitelist + blacklist
			if m.isPhysicalNetworkInterface(iface.Name) {
				// Include physical interface with MAC address
				// HardwareAddr is already a string in gopsutil v4
				macAddr := iface.HardwareAddr
				if macAddr != "" && macAddr != "00:00:00:00:00:00:00:00" && macAddr != "00:00:00:00:00:00" {
					data.WriteString(fmt.Sprintf("Network: %s %s\n", iface.Name, macAddr))
				}
			} else {
				logging.LogDebug(moduleName, fmt.Sprintf("Filtered out virtual network interface: %s", iface.Name))
			}
		}
	} else {
		data.WriteString("Network Information: Error collecting data\n")
	}

	// Detailed network info (platform-specific)
	ctx, cancel = context.WithTimeout(context.Background(), hardwareCollectionTimeout)
	networkInfo := m.collectNetworkInfoWithTimeout(ctx)
	cancel()
	data.WriteString(networkInfo)

	// USB devices (platform-specific)
	ctx, cancel = context.WithTimeout(context.Background(), hardwareCollectionTimeout)
	usbInfo := m.collectUsbInfoWithTimeout(ctx)
	cancel()
	data.WriteString(usbInfo)

	// Host info
	data.WriteString("\n=== Host Information ===\n")
	if hostInfo, err := host.Info(); err == nil {
		data.WriteString(fmt.Sprintf("Host: %s %s %s\n", hostInfo.Hostname, hostInfo.Platform, hostInfo.PlatformVersion))

		// Container detection
		if os.Getenv("IOFOG_DAEMON") != "" {
			data.WriteString(fmt.Sprintf("Container: %s\n", os.Getenv("IOFOG_DAEMON")))
		}
	} else {
		data.WriteString("Host Information: Error collecting data\n")
	}

	result := data.String()
	logging.LogDebug(moduleName, fmt.Sprintf("Collected hardware info length: %d", len(result)))
	logging.LogDebug(moduleName, fmt.Sprintf("Collected hardware info: %s", result)) // TODO: remove in production
	return result
}

// hashData hashes the hardware data
func (m *Manager) hashData(data string) string {
	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// signWithPrivateKey signs the hash with Ed25519 private key
// This matches Java: signWithPrivateKey() method
func (m *Manager) signWithPrivateKey(hash string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Initialize private key if needed
	if m.privateKey == nil {
		if err := m.initializePrivateKey(); err != nil {
			return "", fmt.Errorf("failed to initialize private key: %w", err)
		}
	}

	// Create JWT claims with only the hash value (matching Java implementation)
	claims := jwt.MapClaims{
		"hash": hash,
	}

	// Create JWT with EdDSA algorithm (Ed25519)
	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)

	// Sign token with Ed25519 private key
	tokenString, err := token.SignedString(m.privateKey)
	if err != nil {
		return "", fmt.Errorf("failed to sign token: %w", err)
	}

	logging.LogDebug(moduleName, "Generated JWT with hardware hash")
	return tokenString, nil
}

// initializePrivateKey initializes the Ed25519 private key from JWK
func (m *Manager) initializePrivateKey() error {
	privateKeyStr := m.config.PrivateKey
	if privateKeyStr == "" {
		return errors.New("private key is not configured")
	}

	// Decode base64-encoded JWK
	keyBytes, err := base64.StdEncoding.DecodeString(privateKeyStr)
	if err != nil {
		// Try as raw string
		keyBytes = []byte(privateKeyStr)
	}

	// Parse JWK JSON
	var jwk JWK
	if err := json.Unmarshal(keyBytes, &jwk); err != nil {
		return fmt.Errorf("failed to parse JWK JSON: %w", err)
	}

	// Validate key type
	if jwk.Kty != "OKP" {
		return errors.New("key must be OKP type")
	}

	// Validate curve
	if jwk.Crv != "Ed25519" {
		return errors.New("key must use Ed25519 curve")
	}

	// Decode the private key (d parameter in JWK)
	privateKeyBytes, err := base64.RawURLEncoding.DecodeString(jwk.D)
	if err != nil {
		return fmt.Errorf("failed to decode private key: %w", err)
	}

	// Ed25519 private key seed is 32 bytes
	if len(privateKeyBytes) != ed25519.SeedSize {
		return fmt.Errorf("invalid private key length: expected %d, got %d", ed25519.SeedSize, len(privateKeyBytes))
	}

	// Create Ed25519 private key from seed
	m.privateKey = ed25519.NewKeyFromSeed(privateKeyBytes)

	logging.LogDebug(moduleName, "Successfully initialized Ed25519 signer")
	return nil
}

// performAttestation performs device attestation
// This matches Java: performAttestation() method
// It checks hardware signature, which includes:
// 1. Collecting and signing the hardware signature
// 2. Reading the stored signature from file
// 3. Comparing signatures and handling mismatches
// 4. Writing the signature to file if it's the first run
func (m *Manager) performAttestation() {
	// performAttestation should check hardware signature, which handles all the logic
	// including writing on first run and checking on subsequent runs
	m.checkHardwareSignature()
}


// GetName returns the module name
func (m *Manager) GetName() string {
	return moduleName
}

// GetModuleIndex returns the module index (Edge Guard Manager doesn't have a specific index)
func (m *Manager) GetModuleIndex() int {
	return -1 // Edge Guard Manager is not tracked in status
}

// InstanceConfigUpdated handles configuration updates
// Matching Java: EdgeGuardManager.instanceConfigUpdated()
func (m *Manager) InstanceConfigUpdated() {
	logging.LogDebug(moduleName, "Handling Edge Guard configuration update")
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	currentFreq := m.config.EdgeGuardFrequency
	previousFreq := m.previousFrequency
	
	// If frequency hasn't changed, no action needed
	if currentFreq == previousFreq {
		logging.LogDebug(moduleName, fmt.Sprintf("Edge Guard frequency unchanged: %d", currentFreq))
		return
	}
	
	logging.LogInfo(moduleName, fmt.Sprintf("Edge Guard frequency changed from %d to %d", previousFreq, currentFreq))
	
	// If frequency changed to 0, stop Edge Guard and delete .jwt file
	if currentFreq <= 0 {
		// Stop periodic attestation
		if m.attestationTicker != nil {
			m.attestationTicker.Stop()
			m.attestationTicker = nil
		}
		
		// Delete .jwt file
		if err := hardware.DeleteHardwareSignature(); err != nil {
			logging.LogDebug(moduleName, fmt.Sprintf("No hardware signature file to delete: %v", err))
		} else {
			logging.LogInfo(moduleName, "Edge Guard disabled - hardware signature file deleted")
		}
		
		m.previousFrequency = currentFreq
		logging.LogInfo(moduleName, "Edge Guard Manager disabled (frequency = 0)")
		return
	}
	
	// If frequency changed from 0 to > 0, delete existing .jwt and restart
	if previousFreq <= 0 && currentFreq > 0 {
		// Delete existing .jwt file before starting (fresh start when enabling Edge Guard)
		if err := hardware.DeleteHardwareSignature(); err == nil {
			logging.LogInfo(moduleName, "Deleted existing hardware signature file before enabling Edge Guard (fresh start)")
		} else {
			logging.LogDebug(moduleName, fmt.Sprintf("No existing hardware signature file to delete: %v", err))
		}
		
		// Check if agent is provisioned
		if m.config.IOFogUUID == "" || m.config.PrivateKey == "" {
			logging.LogInfo(moduleName, "Edge Guard cannot start - agent not provisioned (no private key)")
			m.previousFrequency = currentFreq
			return
		}
		
		// Check hardware signature (will create new .jwt file)
		logging.LogInfo(moduleName, "Calling checkHardwareSignature() to create initial hardware signature")
		m.checkHardwareSignature()
		
		// Verify signature file was created
		filePath, err := hardware.GetHardwareSignatureFilePath()
		if err == nil {
			if _, err := os.Stat(filePath); err == nil {
				logging.LogInfo(moduleName, fmt.Sprintf("Hardware signature file verified after frequency change: %s", filePath))
			} else {
				logging.LogWarn(moduleName, fmt.Sprintf("Hardware signature file not found after checkHardwareSignature() call: %s", filePath))
			}
		}
		
		// Start periodic attestation
		duration := time.Duration(currentFreq) * time.Second
		m.attestationTicker = time.NewTicker(duration)
		go m.attestationWorker()
		m.previousFrequency = currentFreq
		logging.LogInfo(moduleName, fmt.Sprintf("Edge Guard Manager enabled with attestation frequency: %d seconds", currentFreq))
		return
	}
	
	// If frequency changed but still > 0, restart with new frequency
	if previousFreq > 0 && currentFreq > 0 {
		// Stop old ticker
		if m.attestationTicker != nil {
			m.attestationTicker.Stop()
		}
		
		// Start new ticker with updated frequency
		duration := time.Duration(currentFreq) * time.Second
		m.attestationTicker = time.NewTicker(duration)
		go m.attestationWorker()
		m.previousFrequency = currentFreq
		logging.LogInfo(moduleName, fmt.Sprintf("Edge Guard Manager frequency updated to: %d seconds", currentFreq))
	}
}

// Platform-specific hardware collection functions with timeout protection
// These functions call the platform-specific implementations

func (m *Manager) collectSystemInfoWithTimeout(ctx context.Context) string {
	done := make(chan string, 1)
	go func() {
		done <- collectSystemInfo(ctx)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		logging.LogError(moduleName, "System information collection timed out", ctx.Err())
		return "\n=== System Hardware ===\nSystem Information: Collection timed out\n"
	}
}

func (m *Manager) collectUsbInfoWithTimeout(ctx context.Context) string {
	done := make(chan string, 1)
	go func() {
		done <- collectUsbInfo(ctx)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		logging.LogError(moduleName, "USB information collection timed out", ctx.Err())
		return "\n=== USB Devices ===\nUSB Information: Collection timed out\n"
	}
}

func (m *Manager) collectPciInfoWithTimeout(ctx context.Context) string {
	done := make(chan string, 1)
	go func() {
		done <- collectPciInfo(ctx)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		logging.LogError(moduleName, "PCI information collection timed out", ctx.Err())
		return "\n=== PCI Devices ===\nPCI Information: Collection timed out\n"
	}
}

func (m *Manager) collectStorageInfoWithTimeout(ctx context.Context) string {
	done := make(chan string, 1)
	go func() {
		done <- collectStorageInfo(ctx)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		logging.LogError(moduleName, "Storage information collection timed out", ctx.Err())
		return "\n=== Storage Devices ===\nStorage Information: Collection timed out\n"
	}
}

func (m *Manager) collectNetworkInfoWithTimeout(ctx context.Context) string {
	done := make(chan string, 1)
	go func() {
		done <- collectNetworkInfo(ctx)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		logging.LogError(moduleName, "Network information collection timed out", ctx.Err())
		return "\n=== Network Interfaces ===\nNetwork Information: Collection timed out\n"
	}
}

func (m *Manager) collectMemoryInfoWithTimeout(ctx context.Context) string {
	done := make(chan string, 1)
	go func() {
		done <- collectMemoryInfo(ctx)
	}()

	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		logging.LogError(moduleName, "Memory information collection timed out", ctx.Err())
		return "\n=== Physical Memory Modules ===\nMemory Information: Collection timed out\n"
	}
}

// Platform-agnostic function declarations
// These are implemented by platform-specific files:
// - hardware_linux.go (Linux implementation)
// - hardware_darwin.go (macOS implementation)
// - hardware_windows.go (Windows implementation)
