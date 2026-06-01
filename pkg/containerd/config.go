//go:build linux && cgo

package edgeletcontainerdd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"

	"github.com/datasance/edgelet/internal/cgroups"
	"github.com/datasance/edgelet/internal/constants"
	"github.com/pelletier/go-toml"
	"github.com/urfave/cli/v2"
)

const (
	defaultContainerdBaseTemplate = `{{ .BaseConfig }}`
)

var (
	containerdTemplateExtensionPath = filepath.Join(constants.EdgeletContainerdLibDir, "config.toml.tmpl")
	containerdLKGSuffix             = ".lkg"
)

// writeConfigFile generates and writes the containerd config.toml to disk.
func writeConfigFile() error {
	if err := os.MkdirAll(filepath.Dir(constants.EdgeletContainerdConfigFile), 0755); err != nil {
		return fmt.Errorf("mkdir for config: %w", err)
	}
	content, err := renderContainerdConfig()
	if err != nil {
		return err
	}
	if err := writeFileAtomically(constants.EdgeletContainerdConfigFile, content, 0644); err != nil {
		return fmt.Errorf("write TOML config atomically: %w", err)
	}
	// PR4 primitive: maintain a successful config snapshot for future rollback wiring.
	if err := writeLastKnownGoodConfig(constants.EdgeletContainerdConfigFile, content); err != nil {
		return fmt.Errorf("write config last-known-good snapshot: %w", err)
	}
	return nil
}

func renderContainerdConfig() ([]byte, error) {
	base, err := renderContainerdBaseConfig()
	if err != nil {
		return nil, err
	}
	return renderContainerdTemplatePipeline(base)
}

func renderContainerdBaseConfig() ([]byte, error) {
	tree, err := toml.TreeFromMap(generateConfig())
	if err != nil {
		return nil, fmt.Errorf("generate TOML tree: %w", err)
	}
	var buf bytes.Buffer
	if _, err := tree.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("render base TOML config: %w", err)
	}
	return buf.Bytes(), nil
}

type containerdTemplateRenderData struct {
	BaseConfig string
}

func renderContainerdTemplatePipeline(baseConfig []byte) ([]byte, error) {
	data := containerdTemplateRenderData{BaseConfig: string(baseConfig)}

	baseTpl, err := template.New("containerd-base").Parse(defaultContainerdBaseTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse base containerd template: %w", err)
	}
	var rendered bytes.Buffer
	if err := baseTpl.Execute(&rendered, data); err != nil {
		return nil, fmt.Errorf("render base containerd template: %w", err)
	}

	extension, err := renderContainerdTemplateExtension(data)
	if err != nil {
		return nil, err
	}
	if len(extension) == 0 {
		return rendered.Bytes(), nil
	}
	if rendered.Len() > 0 && !bytes.HasSuffix(rendered.Bytes(), []byte("\n")) {
		rendered.WriteString("\n")
	}
	rendered.Write(extension)
	if !bytes.HasSuffix(rendered.Bytes(), []byte("\n")) {
		rendered.WriteString("\n")
	}
	return rendered.Bytes(), nil
}

func renderContainerdTemplateExtension(data containerdTemplateRenderData) ([]byte, error) {
	extensionRaw, err := os.ReadFile(containerdTemplateExtensionPath) // #nosec G304 -- controlled internal extension path
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read containerd template extension: %w", err)
	}
	trimmed := strings.TrimSpace(string(extensionRaw))
	if trimmed == "" {
		return nil, nil
	}

	tpl, err := template.New("containerd-extension").Parse(string(extensionRaw))
	if err != nil {
		return nil, fmt.Errorf("parse containerd template extension: %w", err)
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("render containerd template extension: %w", err)
	}
	return buf.Bytes(), nil
}

func writeFileAtomically(path string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("mkdir for atomic write: %w", err)
	}

	tempFile, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	defer func() {
		_ = os.Remove(tempPath)
	}()

	if _, err := tempFile.Write(content); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tempFile.Chmod(perm); err != nil {
		_ = tempFile.Close()
		return fmt.Errorf("chmod temp file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}

func lkgConfigPath(configPath string) string {
	return configPath + containerdLKGSuffix
}

func writeLastKnownGoodConfig(configPath string, content []byte) error {
	return writeFileAtomically(lkgConfigPath(configPath), content, 0644)
}

func readLastKnownGoodConfig(configPath string) ([]byte, error) {
	raw, err := os.ReadFile(lkgConfigPath(configPath)) // #nosec G304 -- controlled internal LKG path
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// generateConfig returns the containerd config map.
// Uses config version 3 (containerd v2+) and registers crun handlers backed by
// crun.
func generateConfig() map[string]any {
	shimRuncPath := filepath.Join(constants.EdgeletContainerdBinDir, "containerd-shim-runc-v2")
	crunPath := filepath.Join(constants.EdgeletContainerdBinDir, "crun")

	policy := cgroups.GetGlobalPolicy()
	cgroupPath := ""
	// Embedded engine uses crun with cgroupfs OCI backend. Host integration
	// (edgelet.service + Delegate=yes) is separate from SystemdCgroup.
	systemdCgroup := false
	if policy != nil && policy.Driver == cgroups.DriverCgroupfs {
		cgroupPath = policy.ContainerdCgroupPath
	}

	crunOptions := map[string]any{
		"BinaryName":    crunPath,
		"SystemdCgroup": systemdCgroup,
	}

	runtimes := map[string]any{
		"crun": map[string]any{
			"runtime_type":                    "io.containerd.runc.v2",
			"runtime_path":                    shimRuncPath,
			"pod_annotations":                 []string{"iofog.network"},
			"container_annotations":           []string{},
			"privileged_without_host_devices": false,
			"privileged_without_host_devices_all_devices_allowed": false,
			"base_runtime_spec": "",
			"cni_conf_dir":      constants.EdgeletCNIConfDir,
			"cni_max_conf_num":  1,
			"snapshotter":       "",
			"sandboxer":         "podsandbox",
			"io_type":           "",
			"options":           crunOptions,
		},
	}
	appendDiscoveredRuntimes(runtimes, systemdCgroup)

	return map[string]any{
		"version":          3,
		"root":             constants.EdgeletContainerdRootDir,
		"state":            constants.EdgeletContainerdStateDir,
		"temp":             "",
		"disabled_plugins": []string{},
		"required_plugins": []string{},
		"oom_score":        0,

		// Allow drop-in config overrides — operators can place *.toml files here.
		"imports": []string{constants.EdgeletContainerdLibDir + "/config.d/*.toml"},

		"grpc": map[string]any{
			"address": constants.EdgeletContainerdSocket,
			"uid":     0,
			"gid":     0,
		},

		"cgroup": map[string]any{
			"path": cgroupPath,
		},

		"timeouts": map[string]any{
			"io.containerd.timeout.bolt.open":         "0s",
			"io.containerd.timeout.metrics.shimstats": "2s",
			"io.containerd.timeout.shim.cleanup":      "5s",
			"io.containerd.timeout.shim.load":         "5s",
			"io.containerd.timeout.shim.shutdown":     "3s",
			"io.containerd.timeout.task.state":        "2s",
		},

		"plugins": map[string]any{
			// --- CRI image plugin ---
			"io.containerd.cri.v1.images": map[string]any{
				"snapshotter":                  "overlayfs",
				"disable_snapshot_annotations": true,
				"discard_unpacked_layers":      false,
				"max_concurrent_downloads":     3,
				"image_pull_progress_timeout":  "2m0s",
				"image_pull_with_sync_fs":      false,
				"stats_collect_period":         120,
				"pinned_images": map[string]any{
					"sandbox": constants.EdgeletSandboxImage,
				},
				"registry": map[string]any{
					"config_path": "",
				},
				"image_decryption": map[string]any{
					"key_model": "node",
				},
			},

			// --- CRI runtime plugin ---
			"io.containerd.cri.v1.runtime": map[string]any{
				"enable_selinux":                         false,
				"selinux_category_range":                 1024,
				"max_container_log_line_size":            16384,
				"disable_apparmor":                       false,
				"restrict_oom_score_adj":                 false,
				"disable_proc_mount":                     false,
				"unset_seccomp_profile":                  "",
				"tolerate_missing_hugetlb_controller":    true,
				"disable_hugetlb_controller":             true,
				"device_ownership_from_security_context": false,
				"ignore_image_defined_volumes":           false,
				"netns_mounts_under_state_dir":           false,
				"enable_unprivileged_ports":              true,
				"enable_unprivileged_icmp":               true,
				"enable_cdi":                             true,
				"drain_exec_sync_io_timeout":             "0s",
				"ignore_deprecation_warnings":            []string{},
				"containerd": map[string]any{
					"default_runtime_name":              "crun",
					"ignore_blockio_not_enabled_errors": false,
					"ignore_rdt_not_enabled_errors":     false,
					"runtimes":                          runtimes,
				},
				// bin_dirs (plural array) replaces the deprecated bin_dir string
				// introduced in containerd v2.1.
				"cni": map[string]any{
					"bin_dirs":              []string{constants.EdgeletCNIPluginsDir},
					"conf_dir":              constants.EdgeletCNIConfDir,
					"max_conf_num":          1,
					"setup_serially":        false,
					"conf_template":         "",
					"ip_pref":               "",
					"use_internal_loopback": false,
				},
			},

			// --- Legacy CRI gRPC shim (stream server only) ---
			"io.containerd.grpc.v1.cri": map[string]any{
				"disable_tcp_service":   true,
				"stream_server_address": "127.0.0.1",
				"stream_server_port":    "0",
				"stream_idle_timeout":   "4h0m0s",
				"enable_tls_streaming":  false,
			},

			// --- Overlayfs snapshotter ---
			"io.containerd.snapshotter.v1.overlayfs": map[string]any{
				"root_path":      "",
				"upperdir_label": false,
				"sync_remove":    false,
				// slow_chown uses sequential chown calls when the process lacks
				// CAP_CHOWN in some restricted environments.
				"slow_chown":    false,
				"mount_options": []string{},
			},

			// --- GC scheduler tuning ---
			"io.containerd.gc.v1.scheduler": map[string]any{
				"pause_threshold":    0.01,
				"deletion_threshold": 0,
				"mutation_threshold": 50,
				"schedule_delay":     "5s",
				"startup_delay":      "200ms",
			},

			// --- Runtime task plugin — declare supported platforms ---
			"io.containerd.runtime.v2.task": map[string]any{
				"platforms": []string{"linux/amd64", "linux/arm64", "linux/arm", "linux/riscv64"},
			},
		},
	}
}

func appendDiscoveredRuntimes(runtimes map[string]any, systemdCgroup bool) {
	catalog := BuildRuntimeCatalog()
	shimRuncPath := filepath.Join(constants.EdgeletContainerdBinDir, "containerd-shim-runc-v2")
	sort.Slice(catalog, func(i, j int) bool {
		return catalog[i].Handler < catalog[j].Handler
	})
	for _, entry := range catalog {
		if entry.Handler == "" || entry.Path == "" {
			continue
		}
		// Baseline crun runtime is statically defined above.
		if entry.Handler == "crun" {
			continue
		}
		if _, exists := runtimes[entry.Handler]; !exists {
			runtimes[entry.Handler] = runtimeClassRuntimeConfig(entry, shimRuncPath, systemdCgroup)
		}
	}
}

func runtimeClassRuntimeConfig(entry RuntimeCatalogEntry, shimRuncPath string, systemdCgroup bool) map[string]any {
	config := map[string]any{
		"runtime_type":                    entry.RuntimeType,
		"pod_annotations":                 []string{"iofog.network"},
		"container_annotations":           []string{},
		"privileged_without_host_devices": false,
		"privileged_without_host_devices_all_devices_allowed": false,
		"base_runtime_spec": "",
		"cni_conf_dir":      constants.EdgeletCNIConfDir,
		"cni_max_conf_num":  1,
		"snapshotter":       "",
		"sandboxer":         "podsandbox",
		"io_type":           "",
	}
	if entry.Family == RuntimeFamilyRunc {
		config["runtime_path"] = shimRuncPath
		config["options"] = map[string]any{
			"BinaryName":    entry.Path,
			"SystemdCgroup": systemdCgroup,
		}
		return config
	}
	config["runtime_path"] = entry.Path
	return config
}

// buildFlags provides the minimal CLI flag set required to boot containerd.
func buildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   constants.EdgeletContainerdConfigFile,
		},
		&cli.StringFlag{
			Name:  "address",
			Value: constants.EdgeletContainerdSocket,
		},
		&cli.StringFlag{
			Name:  "root",
			Value: constants.EdgeletContainerdRootDir,
		},
		&cli.StringFlag{
			Name:  "state",
			Value: constants.EdgeletContainerdStateDir,
		},
		&cli.StringFlag{
			Name:  "log-level",
			Value: "warn",
		},
	}
}
