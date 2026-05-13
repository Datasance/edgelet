//go:build linux

package iofogcontainerd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/eclipse-iofog/agent/internal/constants"
	"github.com/pelletier/go-toml"
	"github.com/urfave/cli/v2"
)

// writeConfigFile generates and writes the containerd config.toml to disk.
func writeConfigFile() error {
	if err := os.MkdirAll(filepath.Dir(constants.IofogContainerdConfigFile), 0755); err != nil {
		return fmt.Errorf("mkdir for config: %w", err)
	}

	tree, err := toml.TreeFromMap(generateConfig())
	if err != nil {
		return fmt.Errorf("generate TOML tree: %w", err)
	}

	f, err := os.Create(constants.IofogContainerdConfigFile) // #nosec G304
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	if _, err := tree.WriteTo(f); err != nil {
		return fmt.Errorf("write TOML config: %w", err)
	}

	return nil
}

// generateConfig returns the containerd config map.
// Uses config version 3 (containerd v2+) and registers runc and, when
// available, the spin/WASM shim. Options are aligned with kubesolo's
// production config for maximum compatibility.
func generateConfig() map[string]any {
	shimRuncPath := filepath.Join(constants.IofogContainerdBinDir, "containerd-shim-runc-v2")
	runcPath := filepath.Join(constants.IofogContainerdBinDir, "runc")
	shimSpinPath := filepath.Join(constants.IofogContainerdBinDir, "containerd-shim-spin")

	runtimes := map[string]any{
		"runc": map[string]any{
			"runtime_type":                    "io.containerd.runc.v2",
			"runtime_path":                    shimRuncPath,
			"pod_annotations":                 []string{"iofog.network"},
			"container_annotations":           []string{},
			"privileged_without_host_devices": false,
			"privileged_without_host_devices_all_devices_allowed": false,
			"base_runtime_spec": "",
			"cni_conf_dir":      constants.IofogManagedCNIConfDir,
			"cni_max_conf_num":  1,
			"snapshotter":       "",
			"sandboxer":         "podsandbox",
			"io_type":           "",
			"options": map[string]any{
				// BinaryName points at our extracted runc so containerd-shim-runc-v2
				// uses our binary rather than any system-installed runc.
				"BinaryName": runcPath,
			},
		},
		"runc-local": map[string]any{
			"runtime_type":                    "io.containerd.runc.v2",
			"runtime_path":                    shimRuncPath,
			"pod_annotations":                 []string{"iofog.network"},
			"container_annotations":           []string{},
			"privileged_without_host_devices": false,
			"privileged_without_host_devices_all_devices_allowed": false,
			"base_runtime_spec": "",
			"cni_conf_dir":      constants.IofogLocalCNIConfDir,
			"cni_max_conf_num":  1,
			"snapshotter":       "",
			"sandboxer":         "podsandbox",
			"io_type":           "",
			"options": map[string]any{
				// BinaryName points at our extracted runc so containerd-shim-runc-v2
				// uses our binary rather than any system-installed runc.
				"BinaryName": runcPath,
			},
		},
	}

	// Register the spin/WASM runtime when the shim is available.
	// Not available on riscv64.
	if spinShimAvailable() {
		runtimes["spin"] = map[string]any{
			"runtime_type":                    "io.containerd.spin.v2",
			"runtime_path":                    shimSpinPath,
			"pod_annotations":                 []string{"iofog.network"},
			"container_annotations":           []string{},
			"privileged_without_host_devices": false,
			"base_runtime_spec":               "",
			"cni_conf_dir":                    constants.IofogManagedCNIConfDir,
			"cni_max_conf_num":                1,
			"sandboxer":                       "podsandbox",
			"options": map[string]any{
				"SystemdCgroup": true,
			},
		}
		runtimes["spin-local"] = map[string]any{
			"runtime_type":                    "io.containerd.spin.v2",
			"runtime_path":                    shimSpinPath,
			"pod_annotations":                 []string{"iofog.network"},
			"container_annotations":           []string{},
			"privileged_without_host_devices": false,
			"base_runtime_spec":               "",
			"cni_conf_dir":                    constants.IofogLocalCNIConfDir,
			"cni_max_conf_num":                1,
			"sandboxer":                       "podsandbox",
			"options": map[string]any{
				"SystemdCgroup": true,
			},
		}
	}

	return map[string]any{
		"version":          3,
		"root":             constants.IofogContainerdRootDir,
		"state":            constants.IofogContainerdStateDir,
		"temp":             "",
		"disabled_plugins": []string{},
		"required_plugins": []string{},
		"oom_score":        0,

		// Allow drop-in config overrides — operators can place *.toml files here.
		"imports": []string{constants.IofogContainerdLibDir + "/config.d/*.toml"},

		"grpc": map[string]any{
			"address": constants.IofogContainerdSocket,
			"uid":     0,
			"gid":     0,
		},

		"cgroup": map[string]any{
			"path": "",
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
					"sandbox": constants.IofogSandboxImage,
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
				"sandbox_image":                          constants.IofogSandboxImage,
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
					"default_runtime_name":              "runc",
					"ignore_blockio_not_enabled_errors": false,
					"ignore_rdt_not_enabled_errors":     false,
					"runtimes":                          runtimes,
				},
				// bin_dirs (plural array) replaces the deprecated bin_dir string
				// introduced in containerd v2.1.
				"cni": map[string]any{
					"bin_dirs":              []string{constants.IofogCNIPluginsDir},
					"conf_dir":              constants.DefaultSystemCNIConfDir,
					"max_conf_num":          2,
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
				"sandbox_image":         constants.IofogSandboxImage,
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

// buildFlags provides the minimal CLI flag set required to boot containerd.
func buildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:    "config",
			Aliases: []string{"c"},
			Value:   constants.IofogContainerdConfigFile,
		},
		&cli.StringFlag{
			Name:  "address",
			Value: constants.IofogContainerdSocket,
		},
		&cli.StringFlag{
			Name:  "root",
			Value: constants.IofogContainerdRootDir,
		},
		&cli.StringFlag{
			Name:  "state",
			Value: constants.IofogContainerdStateDir,
		},
		&cli.StringFlag{
			Name:  "log-level",
			Value: "warn",
		},
	}
}
