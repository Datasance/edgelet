# Embedded Containerd Integration Tests

End-to-end integration tests for `iofog-agentd` with the embedded containerd engine
(`containerEngine: iofog`). All tests run inside a Lima Linux VM on macOS.

## Directory Structure

```
test/embedded/
├── run-all.sh          # Master runner — setup → build → VM → install → test
├── setup.sh            # Install macOS prerequisites via Homebrew
├── build.sh            # Cross-compile linux/arm64 (or amd64) **full**-flavor binaries (`*-full` suffix)
├── vm-start.sh         # Create / start the Lima Ubuntu VM
├── vm-install.sh       # Copy binaries + config into VM, start daemon
├── vm-test.sh          # Run all test assertions inside VM
├── vm-stop.sh          # Stop (and optionally delete) the VM
├── lima-ubuntu.yaml    # Lima VM definition (Ubuntu 24.04, cgroups v2, overlayfs)
└── lib/
    └── log.sh          # Shared colour logging + assert helpers
```

## Quick Start

```bash
# From the agent-go directory:
cd agent-go

# Full pipeline (first run ~5-10 minutes including VM boot + image pull):
./test/embedded/run-all.sh

# On subsequent runs (VM already running, binaries already built):
./test/embedded/run-all.sh --skip-setup --skip-build --skip-start
# or just rerun the test script directly:
./test/embedded/vm-test.sh
```

## Prerequisites

Installed automatically by `setup.sh`:

| Tool | Purpose |
|---|---|
| [Lima](https://github.com/lima-vm/lima) 0.14+ | Linux VM manager for macOS (vzNAT requires 0.14+; macOS 13.0+ for VZ) |
| `jq` | JSON parsing in test assertions |
| `aarch64-unknown-linux-gnu-gcc` | CGO cross-compiler (Apple Silicon) |
| `x86_64-unknown-linux-gnu-gcc` | CGO cross-compiler (Intel Mac) |

**Networking**: The Lima config uses `vzNAT` so the VM gets a host-reachable IP without socket_vmnet or sudoers. Requires Lima 0.14+ and macOS 13.0+ when using `vmType: vz`.

## Options

```
./test/embedded/run-all.sh [options]

  --skip-setup      Skip Homebrew prerequisite installation
  --skip-build      Skip cross-compile (reuse build/iofog-agentd-linux-*)
  --skip-start      Skip VM creation/start (VM must already be running)
  --delete-vm       Delete the VM after tests complete
  --vm-name=NAME    Lima VM name (default: iofog-test)
  --arch=ARCH       Target Linux arch: arm64 | amd64 (default: auto-detect from host)
  --timeout=N       Seconds to wait for VM readiness (default: 300)
  --ci              CI mode — deletes VM on failure
```

## Test Phases

| Phase | What is tested |
|---|---|
| 1 | Extracted embedded binaries (shims, crun, CNI plugins) |
| 2 | containerd socket, health check, `k8s.io` namespace |
| 3 | Managed + local CNI conflists written, network names, bridge names, system symlinks |
| 4 | LocalAPI v3 and CLI checks (`ms ls` table output, `auth whoami`, local `deploy -f`) |
| 5 | Container run, IP forwarding, crun version |
| 6 | CLI: `version`, `info` shows engine=iofog, `config -ce` switching, invalid engine rejected |
| 7 | Chaos gates (restart storm + child crash recovery) |
| 8 | RuntimeClass dual-shim flow (Spin + Edgelet), restart convergence, availableRuntimes, runtime-pinned workloads |

## RuntimeClass dual-shim coverage (Lima arm64)

`vm-test.sh` validates external shim activation through RuntimeClass using these artifacts:

- Spin shim:
  - `https://github.com/spinframework/containerd-shim-spin/releases/download/v0.24.0/containerd-shim-spin-v2-linux-aarch64.tar.gz`
- Edgelet shim:
  - `https://github.com/Datasance/containerd-shim-edgelet/releases/download/v0.1.0/containerd-shim-edgelet-wasm-v2-aarch64-linux-gnu.tar.gz`

Coverage includes:

- shim binary install into embedded runtime bin directory
- RuntimeClass validate/apply via CLI/API (sync success or async accepted + poll to terminal)
- controlled containerd restart convergence after apply
- `availableRuntimes` visibility for class and class-local entries
- runtime-pinned local workloads running for each RuntimeClass
- RuntimeClass delete guard rejection while runtime-pinned workload exists
- RuntimeClass delete success after removing dependent workloads (sync success or async accepted + poll to terminal)
- runtime entries removed from effective runtime map after delete convergence

## Individual Scripts

```bash
# Just install prerequisites
./test/embedded/setup.sh

# Just build binaries (auto-detects Apple Silicon → linux/arm64)
./test/embedded/build.sh
./test/embedded/build.sh --arch=amd64

# Just start the VM
./test/embedded/vm-start.sh

# Just install agent in the VM
./test/embedded/vm-install.sh

# Just run tests (VM must already have agent installed)
./test/embedded/vm-test.sh

# Stop the VM (keep data)
./test/embedded/vm-stop.sh

# Stop and delete the VM
./test/embedded/vm-stop.sh --delete
```

## Using the Desktop App Instead

If the desktop app is running with the Lima VM already active, you can run
the test script against the same VM:

```bash
./test/embedded/vm-test.sh --vm-name=iofog
```

## Connecting to the VM directly

```bash
# Open a shell
limactl shell iofog-test

# View daemon logs
limactl shell iofog-test -- sudo journalctl -fu iofog-agentd

# Direct containerd access via ctr
limactl shell iofog-test -- sudo ctr \
    --address /run/iofog-agent/containerd.sock \
    --namespace iofog \
    images list
```
