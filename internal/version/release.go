package version

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultBackupDir       = "/var/backups/edgelet"
	defaultInstallScript   = "/usr/share/edgelet/install.sh"
	defaultInstallReceipt  = "/var/backups/edgelet/install-receipt"
	defaultPreviousRelease = "/var/backups/edgelet/previous-release"
	defaultCacheDir        = "/var/backups/edgelet/cache"
	defaultGitHubRepo      = "eclipse-iofog/edgelet"
	githubLatestReleaseURL = "https://api.github.com/repos/%s/releases/latest"
)

// InstallReceipt holds parsed install-receipt v1 fields.
type InstallReceipt struct {
	InstalledVersion string
	OS               string
	Arch             string
	ContainerEngine  string
	SourceURL        string
	InstalledAt      string
	InstallMethod    string
	BinarySHA256     string
}

// PreviousRelease holds parsed previous-release fields.
type PreviousRelease struct {
	PreviousVersion         string
	PreviousOS              string
	PreviousArch            string
	PreviousContainerEngine string
	PreviousDownloadURL     string
	PreviousBinarySHA256    string
	ConfigBackupPath        string
}

// ReleaseManager reads OTA state from install.sh receipt files and GitHub releases.
type ReleaseManager struct {
	backupDir      string
	cacheDir       string
	installScript  string
	receiptFile    string
	previousFile   string
	githubRepo     string
	httpClient     *http.Client
	runningVersion func() string
	githubRepoEnv  func() string
}

// ReleaseManagerOption configures a ReleaseManager.
type ReleaseManagerOption func(*ReleaseManager)

// WithPaths overrides default on-disk OTA paths (tests).
func WithPaths(installScript, receiptFile, previousFile, cacheDir string) ReleaseManagerOption {
	return func(rm *ReleaseManager) {
		if installScript != "" {
			rm.installScript = installScript
		}
		if receiptFile != "" {
			rm.receiptFile = receiptFile
		}
		if previousFile != "" {
			rm.previousFile = previousFile
		}
		if cacheDir != "" {
			rm.cacheDir = cacheDir
		}
	}
}

// WithHTTPClient overrides the HTTP client (tests).
func WithHTTPClient(client *http.Client) ReleaseManagerOption {
	return func(rm *ReleaseManager) {
		if client != nil {
			rm.httpClient = client
		}
	}
}

// WithRunningVersion overrides the running agent version source (tests).
func WithRunningVersion(fn func() string) ReleaseManagerOption {
	return func(rm *ReleaseManager) {
		if fn != nil {
			rm.runningVersion = fn
		}
	}
}

// NewReleaseManager constructs a ReleaseManager with optional overrides.
func NewReleaseManager(opts ...ReleaseManagerOption) *ReleaseManager {
	rm := &ReleaseManager{
		backupDir:      defaultBackupDir,
		cacheDir:       defaultCacheDir,
		installScript:  defaultInstallScript,
		receiptFile:    defaultInstallReceipt,
		previousFile:   defaultPreviousRelease,
		githubRepo:     defaultGitHubRepo,
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		runningVersion: GetVersion,
		githubRepoEnv:  func() string { return os.Getenv("EDGELET_GITHUB_REPO") },
	}
	for _, opt := range opts {
		opt(rm)
	}
	return rm
}

// InstallScriptPath returns the bundled install script path.
func (rm *ReleaseManager) InstallScriptPath() string {
	return rm.installScript
}

// InstallScriptExists reports whether the bundled install script is present.
func (rm *ReleaseManager) InstallScriptExists() bool {
	info, err := os.Stat(rm.installScript)
	return err == nil && !info.IsDir()
}

// ReadInstallReceipt parses install-receipt v1 if present.
func (rm *ReleaseManager) ReadInstallReceipt() (*InstallReceipt, error) {
	kv, err := readKVFile(rm.receiptFile)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &InstallReceipt{
		InstalledVersion: kv["installed_version"],
		OS:               kv["os"],
		Arch:             kv["arch"],
		ContainerEngine:  kv["container_engine"],
		SourceURL:        kv["source_url"],
		InstalledAt:      kv["installed_at"],
		InstallMethod:    kv["install_method"],
		BinarySHA256:     kv["binary_sha256"],
	}, nil
}

// ReadPreviousRelease parses previous-release if present.
func (rm *ReleaseManager) ReadPreviousRelease() (*PreviousRelease, error) {
	kv, err := readKVFile(rm.previousFile)
	if err != nil {
		return nil, err
	}
	if len(kv) == 0 {
		return nil, nil
	}
	return &PreviousRelease{
		PreviousVersion:         kv["previous_version"],
		PreviousOS:              kv["previous_os"],
		PreviousArch:            kv["previous_arch"],
		PreviousContainerEngine: kv["previous_container_engine"],
		PreviousDownloadURL:     kv["previous_download_url"],
		PreviousBinarySHA256:    kv["previous_binary_sha256"],
		ConfigBackupPath:        kv["config_backup_path"],
	}, nil
}

// PreviousReleaseExists reports whether a previous-release record is on disk.
func (rm *ReleaseManager) PreviousReleaseExists() bool {
	info, err := os.Stat(rm.previousFile)
	return err == nil && !info.IsDir()
}

// GetInstalledVersion returns receipt installed_version or the running build version.
func (rm *ReleaseManager) GetInstalledVersion() string {
	receipt, err := rm.ReadInstallReceipt()
	if err == nil && receipt != nil && strings.TrimSpace(receipt.InstalledVersion) != "" {
		return normalizeVersion(receipt.InstalledVersion)
	}
	return normalizeVersion(rm.runningVersion())
}

// GetCandidateVersion resolves the controller target from action data or GitHub latest.
func (rm *ReleaseManager) GetCandidateVersion(actionData map[string]any) (string, error) {
	if target := targetVersionFromAction(actionData); target != "" {
		return normalizeVersion(target), nil
	}
	return rm.fetchLatestReleaseTag()
}

// HasCachedBinary reports whether rollback cache contains the previous binary.
func (rm *ReleaseManager) HasCachedBinary(version, osName, arch string) bool {
	version = strings.TrimSpace(version)
	osName = strings.TrimSpace(osName)
	arch = strings.TrimSpace(arch)
	if version == "" || osName == "" || arch == "" {
		return false
	}
	path := filepath.Join(rm.cacheDir, fmt.Sprintf("edgelet-%s-%s-%s", version, osName, arch))
	if osName == "windows" {
		path += ".exe"
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// IsPreviousDownloadReachable checks whether previous_download_url responds.
func (rm *ReleaseManager) IsPreviousDownloadReachable(url string) bool {
	url = strings.TrimSpace(url)
	if url == "" {
		return false
	}
	if strings.HasPrefix(url, "file://") {
		path := strings.TrimPrefix(url, "file://")
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	}
	req, err := http.NewRequest(http.MethodHead, url, nil)
	if err != nil {
		return false
	}
	resp, err := rm.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

func (rm *ReleaseManager) fetchLatestReleaseTag() (string, error) {
	repo := strings.TrimSpace(rm.githubRepoEnv())
	if repo == "" {
		repo = rm.githubRepo
	}
	url := fmt.Sprintf(githubLatestReleaseURL, repo)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := rm.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("github releases/latest: %s (%s)", resp.Status, strings.TrimSpace(string(body)))
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	tag := strings.TrimSpace(release.TagName)
	if tag == "" {
		return "", errors.New("github releases/latest: empty tag_name")
	}
	return normalizeVersion(tag), nil
}

func readKVFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- release metadata path from build-time constant or env
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	kv := make(map[string]string)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		kv[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return kv, nil
}

// TargetVersionFromAction resolves the controller target version with semver precedence.
func TargetVersionFromAction(actionData map[string]any) string {
	if actionData == nil {
		return ""
	}
	if raw, ok := actionData["semver"].(string); ok {
		if v := strings.TrimSpace(raw); v != "" {
			return v
		}
	}
	for _, key := range []string{"version", "targetVersion", "target"} {
		if raw, ok := actionData[key].(string); ok {
			if v := strings.TrimSpace(raw); v != "" {
				return v
			}
		}
	}
	return ""
}

func targetVersionFromAction(actionData map[string]any) string {
	return TargetVersionFromAction(actionData)
}

func normalizeVersion(version string) string {
	return strings.TrimPrefix(strings.TrimSpace(version), "v")
}

// versionForInstallScript formats a release tag for install.sh --version= (always v-prefixed).
func versionForInstallScript(version string) string {
	version = strings.TrimSpace(version)
	if version == "" || strings.EqualFold(version, "latest") {
		return version
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}
