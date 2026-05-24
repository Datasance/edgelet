package version

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/datasance/edgelet/internal/utils"
	"github.com/datasance/edgelet/internal/utils/logging"
)

const (
	moduleName              = "Version Handler"
	packageName             = "iofog-agent"
	maxRestartingTimeout    = "60"
	getLinuxDistributionCmd = "grep = /etc/os-release | awk -F\"[=]\" '{print $2}' | sed -n 1p"
)

// VersionCommand represents a version change command
type VersionCommand string

const (
	VersionCommandUpgrade  VersionCommand = "UPGRADE"
	VersionCommandRollback VersionCommand = "ROLLBACK"
)

// ParseVersionCommand parses version command from JSON
func ParseVersionCommand(actionData map[string]interface{}) (VersionCommand, error) {
	cmdStr, ok := actionData["command"].(string)
	if !ok {
		return "", fmt.Errorf("command not found in action data")
	}

	cmd := VersionCommand(strings.ToUpper(cmdStr))
	switch cmd {
	case VersionCommandUpgrade, VersionCommandRollback:
		return cmd, nil
	default:
		return "", fmt.Errorf("unknown version command: %s", cmdStr)
	}
}

// Handler handles version management operations
type Handler struct {
	distributionName string
	packageManager   PackageManager
}

var (
	instance *Handler
	once     sync.Once
)

// GetInstance returns the singleton Version Handler instance
func GetInstance() *Handler {
	once.Do(func() {
		instance = &Handler{}
		instance.init()
	})
	return instance
}

// init initializes the version handler
func (h *Handler) init() {
	// Check if running in container
	iofogDaemon := os.Getenv("EDGELET_DAEMON")
	if strings.ToLower(iofogDaemon) == "container" {
		h.distributionName = "container"
		h.packageManager = &ContainerPackageManager{}
		return
	}

	// Detect Linux distribution
	if runtime.GOOS == "linux" {
		h.distributionName = h.getDistributionName()
		h.packageManager = h.detectPackageManager()
	} else if runtime.GOOS == "windows" {
		h.distributionName = "windows"
		h.packageManager = &WindowsPackageManager{} // TODO: implement
	} else {
		h.distributionName = "unknown"
		h.packageManager = &UnsupportedPackageManager{}
	}
}

// getDistributionName gets the Linux distribution name
func (h *Handler) getDistributionName() string {
	stdout, _, err := utils.ExecuteCommand(getLinuxDistributionCmd)
	if err != nil || stdout == "" {
		return ""
	}

	distName := strings.TrimSpace(stdout)
	return strings.ToLower(distName)
}

// PackageManager interface for different package managers
type PackageManager interface {
	GetInstalledVersion() (string, error)
	GetCandidateVersion() (string, error)
	UpdateRepository() (bool, error)
	GetScript(command VersionCommand) string
}

// AptPackageManager handles apt-based distributions (Ubuntu, Debian, Raspbian)
type AptPackageManager struct{}

func (a *AptPackageManager) GetInstalledVersion() (string, error) {
	// Get dev version suffix first
	devVersion, err := a.getDevVersion()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("apt-cache policy %s%s | grep Installed | awk '{print $2}'", packageName, devVersion)
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (a *AptPackageManager) GetCandidateVersion() (string, error) {
	devVersion, err := a.getDevVersion()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("apt-cache policy %s%s | grep Candidate | awk '{print $2}'", packageName, devVersion)
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (a *AptPackageManager) UpdateRepository() (bool, error) {
	// Check lock files
	lockCmd := "cat /var/lib/apt/lists/lock /var/cache/apt/archives/lock"
	stdout, _, err := utils.ExecuteCommand(lockCmd)
	if err == nil && stdout != "" {
		logging.LogWarn(moduleName, "Unable to update package repository. Another app is currently holding package manager lock")
		return false, nil
	}

	// Update repository
	updateCmd := "apt-get update -y"
	_, _, err = utils.ExecuteCommand(updateCmd)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (a *AptPackageManager) GetScript(command VersionCommand) string {
	switch command {
	case VersionCommandUpgrade:
		return "upgrade"
	case VersionCommandRollback:
		return "rollback"
	default:
		return ""
	}
}

func (a *AptPackageManager) getDevVersion() (string, error) {
	cmd := fmt.Sprintf("(apt-cache policy %s-dev && apt-cache policy %s) | grep -A1 ^iofog | awk '$2 ~ /^[0-9]/ {print a}{a=$0}' | sed -e 's/iofog-agent\\(.*\\):/\\1/'", packageName, packageName)
	stdout, _, _ := utils.ExecuteCommand(cmd)
	if stdout == "" {
		return "", nil
	}

	return strings.TrimSpace(stdout), nil
}

// DnfPackageManager handles dnf-based distributions (Fedora)
type DnfPackageManager struct{}

func (d *DnfPackageManager) GetInstalledVersion() (string, error) {
	devVersion, err := d.getDevVersion()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("dnf --showduplicates list installed %s%s | grep iofog | awk '{print $2}'", packageName, devVersion)
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (d *DnfPackageManager) GetCandidateVersion() (string, error) {
	devVersion, err := d.getDevVersion()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("dnf --refresh list && dnf --showduplicates list %s%s | grep iofog | awk '{print $2}' | sed -n \\$p\\", packageName, devVersion)
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (d *DnfPackageManager) UpdateRepository() (bool, error) {
	// Check lock file
	lockCmd := "cat /var/cache/dnf/metadata_lock.pid"
	stdout, _, err := utils.ExecuteCommand(lockCmd)
	if err == nil && stdout != "" {
		logging.LogWarn(moduleName, "Unable to update package repository. Another app is currently holding package manager lock")
		return false, nil
	}

	// Update repository
	updateCmd := "dnf update -y"
	_, _, err = utils.ExecuteCommand(updateCmd)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (d *DnfPackageManager) GetScript(command VersionCommand) string {
	switch command {
	case VersionCommandUpgrade:
		return "upgrade"
	case VersionCommandRollback:
		return "rollback"
	default:
		return ""
	}
}

func (d *DnfPackageManager) getDevVersion() (string, error) {
	cmd := fmt.Sprintf("(dnf --showduplicates list installed %s-dev && dnf --showduplicates list installed %s) | grep iofog | awk '{print $1}' | sed -e 's/iofog-agent\\(.*\\).noarch/\\1/'", packageName, packageName)
	stdout, _, _ := utils.ExecuteCommand(cmd)
	if stdout == "" {
		return "", nil
	}

	return strings.TrimSpace(stdout), nil
}

// YumPackageManager handles yum-based distributions (CentOS, Amazon Linux)
type YumPackageManager struct{}

func (y *YumPackageManager) GetInstalledVersion() (string, error) {
	devVersion, err := y.getDevVersion()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("yum --showduplicates list installed | grep %s%s | awk '{print $2}'", packageName, devVersion)
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (y *YumPackageManager) GetCandidateVersion() (string, error) {
	devVersion, err := y.getDevVersion()
	if err != nil {
		return "", err
	}

	cmd := fmt.Sprintf("yum --refresh list && yum --showduplicates list | grep %s%s | awk '{print $2}' | sed -n \\$p\\", packageName, devVersion)
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (y *YumPackageManager) UpdateRepository() (bool, error) {
	// Check lock file
	lockCmd := "cat /var/run/yum.pid"
	stdout, _, err := utils.ExecuteCommand(lockCmd)
	if err == nil && stdout != "" {
		logging.LogWarn(moduleName, "Unable to update package repository. Another app is currently holding package manager lock")
		return false, nil
	}

	// Update repository
	updateCmd := "yum update -y"
	_, _, err = utils.ExecuteCommand(updateCmd)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (y *YumPackageManager) GetScript(command VersionCommand) string {
	switch command {
	case VersionCommandUpgrade:
		return "upgrade"
	case VersionCommandRollback:
		return "rollback"
	default:
		return ""
	}
}

func (y *YumPackageManager) getDevVersion() (string, error) {
	cmd := fmt.Sprintf("(yum --showduplicates list installed %s-dev && yum --showduplicates list installed %s) | grep iofog | awk '{print $1}' | sed -e 's/iofog-agent\\(.*\\).noarch/\\1/'", packageName, packageName)
	stdout, _, _ := utils.ExecuteCommand(cmd)
	if stdout == "" {
		return "", nil
	}

	return strings.TrimSpace(stdout), nil
}

// ContainerPackageManager handles containerized agents
type ContainerPackageManager struct{}

func (c *ContainerPackageManager) GetInstalledVersion() (string, error) {
	cmd := "iofog-agent version | grep -oP 'Agent\\s+\\K[0-9]+(\\.[0-9]+){2,3}(?=\\s|$)'"
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (c *ContainerPackageManager) GetCandidateVersion() (string, error) {
	// Get latest version from GitHub releases
	cmd := "curl -s https://api.github.com/repos/Datasance/Agent/releases | grep '\"tag_name\":' | grep -v '\"latest\"' | awk -F '\"' '{print $4}' | awk '{print substr($0, 2)}' | head -n 1"
	stdout, _, err := utils.ExecuteCommand(cmd)
	if err != nil || stdout == "" {
		return "", err
	}

	return strings.TrimSpace(stdout), nil
}

func (c *ContainerPackageManager) UpdateRepository() (bool, error) {
	// No-op for container
	return true, nil
}

func (c *ContainerPackageManager) GetScript(_ VersionCommand) string {
	// Container doesn't support version changes via script
	return ""
}

// WindowsPackageManager handles Windows (not implemented)
type WindowsPackageManager struct{}

func (w *WindowsPackageManager) GetInstalledVersion() (string, error) {
	return "", fmt.Errorf("Windows version management not implemented")
}

func (w *WindowsPackageManager) GetCandidateVersion() (string, error) {
	return "", fmt.Errorf("Windows version management not implemented")
}

func (w *WindowsPackageManager) UpdateRepository() (bool, error) {
	return false, fmt.Errorf("Windows version management not implemented")
}

func (w *WindowsPackageManager) GetScript(_ VersionCommand) string {
	return ""
}

// UnsupportedPackageManager handles unsupported platforms
type UnsupportedPackageManager struct{}

func (u *UnsupportedPackageManager) GetInstalledVersion() (string, error) {
	return "", fmt.Errorf("unsupported platform")
}

func (u *UnsupportedPackageManager) GetCandidateVersion() (string, error) {
	return "", fmt.Errorf("unsupported platform")
}

func (u *UnsupportedPackageManager) UpdateRepository() (bool, error) {
	return false, fmt.Errorf("unsupported platform")
}

func (u *UnsupportedPackageManager) GetScript(_ VersionCommand) string {
	return ""
}

// detectPackageManager detects the appropriate package manager based on distribution
func (h *Handler) detectPackageManager() PackageManager {
	distName := h.distributionName

	if strings.Contains(distName, "ubuntu") ||
		strings.Contains(distName, "debian") ||
		strings.Contains(distName, "raspbian") {
		return &AptPackageManager{}
	} else if strings.Contains(distName, "fedora") {
		return &DnfPackageManager{}
	} else if strings.Contains(distName, "centos") ||
		strings.Contains(distName, "amazon") {
		return &YumPackageManager{}
	}
	logging.LogWarn(moduleName, "it looks like your distribution is not supported")
	return &UnsupportedPackageManager{}
}

// ChangeVersion performs version change operation
func (h *Handler) ChangeVersion(actionData map[string]interface{}) error {
	logging.LogInfo(moduleName, "Start performing change version operation, received from ioFog controller")

	// Check if running in container
	iofogDaemon := os.Getenv("EDGELET_DAEMON")
	if strings.ToLower(iofogDaemon) == "container" {
		logging.LogWarn(moduleName, "IoFog Agent daemon is running inside container, please upgrade/rollback version via `potctl`")
		return nil
	}

	if runtime.GOOS == "windows" {
		logging.LogWarn(moduleName, "Windows version management not implemented")
		return nil
	}

	// Parse version command
	command, err := ParseVersionCommand(actionData)
	if err != nil {
		logging.LogError(moduleName, "Error performing change version operation: Invalid command", err)
		return err
	}

	provisionKey, _ := actionData["provisionKey"].(string)

	// Validate and execute
	if h.isValidChangeVersionOperation(command) {
		if err := h.executeChangeVersionScript(command, provisionKey); err != nil {
			return err
		}
	}

	logging.LogInfo(moduleName, "Finished performing change version operation, received from ioFog controller")
	return nil
}

// executeChangeVersionScript executes the version change script
func (h *Handler) executeChangeVersionScript(command VersionCommand, provisionKey string) error {
	logging.LogInfo(moduleName, "Start executing sh script to change iofog version")

	script := h.packageManager.GetScript(command)
	if script == "" {
		return fmt.Errorf("no script available for command: %s", command)
	}

	// Execute version controller jar
	// Note: In Go, we might need to implement this differently
	// For now, we'll use the same approach as Java
	cmd := exec.Command("java", "-jar", "/usr/bin/iofog-agentvc.jar", script, provisionKey, maxRestartingTimeout) // #nosec G204 -- binary is hardcoded constant; args are from internal package manager

	if err := cmd.Start(); err != nil {
		logging.LogError(moduleName, "Error executing sh script to change version", err)
		return err
	}

	logging.LogInfo(moduleName, "Finished executing sh script to change iofog version")
	return nil
}

// isValidChangeVersionOperation checks if the version change operation is valid
func (h *Handler) isValidChangeVersionOperation(command VersionCommand) bool {
	switch command {
	case VersionCommandUpgrade:
		return h.IsReadyToUpgrade()
	case VersionCommandRollback:
		return h.IsReadyToRollback()
	default:
		return false
	}
}

// IsReadyToUpgrade checks if the system is ready to upgrade
func (h *Handler) IsReadyToUpgrade() bool {
	logging.LogDebug(moduleName, "Checking if ready to upgrade")

	iofogDaemon := os.Getenv("EDGELET_DAEMON")
	isContainer := strings.ToLower(iofogDaemon) == "container"

	var isReady bool
	if isContainer {
		// For container, only check if versions are different
		isReady = h.areNotVersionsSame()
	} else {
		// For non-container, check all conditions
		if runtime.GOOS == "windows" {
			return false
		}

		repoUpdated, err := h.packageManager.UpdateRepository()
		if err != nil || !repoUpdated {
			return false
		}

		isReady = h.areNotVersionsSame()
	}

	logging.LogDebug(moduleName, fmt.Sprintf("Is ready to upgrade: %v", isReady))
	return isReady
}

// areNotVersionsSame checks if installed and candidate versions are different
func (h *Handler) areNotVersionsSame() bool {
	installed, err1 := h.packageManager.GetInstalledVersion()
	candidate, err2 := h.packageManager.GetCandidateVersion()

	if err1 != nil || err2 != nil {
		return false
	}

	return installed != candidate
}

// IsReadyToRollback checks if the system is ready to rollback
func (h *Handler) IsReadyToRollback() bool {
	logging.LogDebug(moduleName, "Checking is ready to rollback")

	// Determine backups directory based on OS
	var backupsDir string
	snapCommon := os.Getenv("SNAP_COMMON")
	if snapCommon == "" {
		snapCommon = ""
	}
	if runtime.GOOS == "windows" {
		backupsDir = filepath.Join(snapCommon, "./var/backups/iofog-agent")
	} else {
		backupsDir = filepath.Join(snapCommon, "/var/backups/iofog-agent")
	}

	// Check if backups directory exists and has files
	files, err := os.ReadDir(backupsDir)
	if err != nil {
		return false
	}

	isReady := len(files) > 0
	logging.LogDebug(moduleName, fmt.Sprintf("Is ready to rollback: %v", isReady))
	return isReady
}
