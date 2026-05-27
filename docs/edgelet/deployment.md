# Edgelet deployment

This guide covers production installation of Edgelet on Linux edge nodes.

## Prerequisites

- Linux (Ubuntu 20.04+, RHEL/CentOS 8+, Debian 11+)
- Root or sudo access
- Network connectivity to the ioFog Controller
- For **`containerEngine: docker` or `podman`:** Docker 26.10+ or Podman 3.0+ on the host
- For **`containerEngine: edgelet` (default):** no external container runtime required

## Installation

Edgelet ships as **tarball + `install.sh` only** (no DEB/RPM in this product line).

```bash
curl -fsSL https://raw.githubusercontent.com/datasance/agent/main/install.sh | sudo sh
# or with a local tarball:
sudo sh install.sh --tarball-path=edgelet-linux-amd64.tar.gz
```

Tarball names: **`edgelet-linux-<arch>.tar.gz`** (versioned: `edgelet-<VERSION>-linux-<arch>.tar.gz`)

| Flag | Purpose |
|------|---------|
| `--container-engine=docker\|podman\|edgelet` | Sets `containerEngine` in installed config (default on linux: **edgelet**) |
| `--airgap` | Offline install; requires `--tarball-path` |
| `--upgrade` / `--rollback` | In-place OTA |
| `--flavor=full\|lite` | **Deprecated** — ignored (unified linux tarball since Plan 7) |

Supported init systems: **systemd**, OpenRC, SysV init, s6, runit, upstart.

Packaging details: [../../packaging/PACKAGING-STRUCTURE.md](../../packaging/PACKAGING-STRUCTURE.md).

---

## Daemon and systemd

Production linux uses the **thin** binary as the systemd entry point. The unit never invokes the fat runtime path directly.

```bash
edgelet daemon          # thin: extract if edgelet engine → exec fat … daemon
systemctl start edgelet # ExecStart=/usr/local/bin/edgelet daemon
```

Unit file: `packaging/systemd/edgelet.service` — `ExecStart=/usr/local/bin/edgelet daemon`.

When `containerEngine` is **docker** or **podman**, add a systemd drop-in so edgelet starts after the external engine socket is available (recommended in addition to in-process init retries):

```ini
[Unit]
After=docker.service
# or: After=podman.service
```

On first boot edgelet retries docker/podman socket connection with exponential backoff (2s initial, 30s cap, 12 attempts, ~4m worst-case wait). If the socket is still unavailable, the daemon stays up in **WARNING** status and keeps retrying in the background until the engine is ready.

On first start with **`containerEngine: edgelet`** (or after upgrading the thin binary), `edgelet daemon` **lazy-extracts** the embedded zstd bundle (k3s-style: content-addressed by the thin binary’s embed hash). Subsequent starts skip re-extract when `data/<embed-hash>/` is already ready; `data/current` is synchronized to that directory.

**Operator CLI** (`edgelet ms`, `edgelet deploy`, `edgelet version`, …) runs in the **thin** process and does **not** trigger extract.

**Break-glass:** Run the fat runtime directly when debugging extract or dispatch issues:

```bash
/var/lib/edgelet/data/current/bin/edgelet daemon
# or, when you know the embed hash:
/var/lib/edgelet/data/<sha256-prefix>/bin/edgelet daemon
```

### Embedded runtime data layout (`containerEngine: edgelet`)

Under `/var/lib/edgelet/data/`:

| Path | Role |
|------|------|
| `<sha256-prefix>/` | Extract of the zstd bundle baked into the **thin** binary (authoritative tree) |
| `current` | Symlink to the active `<sha256-prefix>/` (updated on extract / promote) |
| `previous` | Symlink to the last `current` target before rotation (operator hint; not auto-used at runtime) |
| `cni/` | Stable CNI multicall symlinks into the active bundle |
| `.lock` | Extract flock |

Extract checks `/var/lib/edgelet/data/<embed-hash>/bin/edgelet` first (same model as k3s `data/<hash>/bin/k3s`), then updates `current` / `previous`.

---

## Runtime engine selection

Set **`containerEngine`** in `/etc/edgelet/config.yaml`. The linux binary supports all values at runtime; there is no compile-time full/lite split.

| `containerEngine` | Extract bundle? | Host requirement |
|-------------------|-----------------|------------------|
| **`edgelet`** (default) | Yes, on first daemon start | None |
| `docker` | No | Docker socket at `dockerUrl` |
| `podman` | No | Podman socket at `dockerUrl` |

Verify capability:

```bash
edgelet --version
edgelet system version -o json | jq '{embeddedEngine, allowedEngines, containerEngine}'
```

### OTA upgrade (thin binary)

Fleet OTA typically replaces only `/usr/local/bin/edgelet` (thin) and restarts `edgelet.service`. When using `containerEngine: edgelet`, the new thin embed carries a new SHA256 prefix. On the first `edgelet daemon` after restart:

1. Extract targets **`/var/lib/edgelet/data/<new-hash>/`** (not `current` alone).
2. If that tree is missing, unpack, verify `bin/`, rotate `current` → `previous`, and point `current` at the new tree.
3. Older hash directories remain on disk until manually removed (~2× bundle size per retained generation).

Logs (info, module `Data`) when `current` still pointed at an older hash:

```text
Embedded bundle hash mismatch (installed=<old> embedded=<new>); re-extracting
Preparing data dir /var/lib/edgelet/data/<new-hash>   # when a full unpack runs
```

Verify after upgrade:

```bash
readlink -f /var/lib/edgelet/data/current
edgelet version   # embed hash should match basename of current
```

Greenfield reinstall is required when migrating from pre–Plan 6 single-ELF layouts or obsolete `-full`/`-lite` tarballs.

### Rollback

#### Release rollback (`install.sh --rollback`)

Rolls back the **installed thin binary** (and optionally config) using install backup metadata (`previous_version`, etc.). Restart `edgelet.service` after rollback.

This does **not** delete extracted bundle directories under `/var/lib/edgelet/data/`. After rollback, extract follows the **rolled-back** thin binary’s embed hash.

#### Runtime bundle rollback (`containerEngine: edgelet`)

There is **no automatic** revert to `data/previous` if the new daemon fails (k3s does not auto-revert either). `previous` points at the last rotated tree for operator use.

**Supported manual rollback** — downgrade to a thin binary whose embed hash still has a ready tree on disk:

1. Install the older thin edgelet (or `install.sh --rollback` when it restores that release).
2. `systemctl restart edgelet`.
3. Extract reuses `data/<that-hash>/` if `bin/edgelet` is still valid; `current` is promoted to that directory.

If the old tree was deleted, reinstall that edgelet version so the matching bundle can be extracted again.

**Emergency symlink** (same hash still on disk):

```bash
systemctl stop edgelet
ln -sfn /var/lib/edgelet/data/<hash> /var/lib/edgelet/data/current
/var/lib/edgelet/data/current/bin/edgelet daemon
```

Prefer a version-matched thin install over hand-edited symlinks.

#### Pruning old bundles

Only when rollback to that generation is not needed:

```bash
systemctl stop edgelet
rm -rf /var/lib/edgelet/data/<unused-hash>
# keep current (and previous if you may revert one step)
```

---

## Configuration

Primary config: `/etc/edgelet/config.yaml`

```yaml
controller: https://controller.example.com
deviceName: edge-device-01
containerEngine: edgelet          # linux default
dockerUrl: unix:///run/edgelet/containerd.sock
logLevel: INFO
logDiskDirectory: /var/log/edgelet
profiles:
  production:
    containerEngine: edgelet
    dockerUrl: unix:///run/edgelet/containerd.sock
```

For docker/podman:

```yaml
containerEngine: docker
dockerUrl: unix:///var/run/docker.sock
```

On first start the daemon auto-creates `/etc/edgelet/edgelet-api` (CLI bearer token for EdgeletAPI).

---

## EdgeletAPI PKI

TLS material under `/etc/edgelet/`:

| File | Purpose |
|------|---------|
| `edgeletapi-ca.crt` | CA for CLI trust |
| `edgeletapi-server.crt` | Server certificate |
| `edgeletapi-server.key` | Server private key |

The CLI reads the bearer token from `/etc/edgelet/edgelet-api` and trusts `edgeletapi-ca.crt`.

---

## Provisioning

Register the edge node with the Controller:

```bash
edgelet provision --controller-url https://controller.example.com --provision-key <KEY>
edgelet system status
```

Deprovision:

```bash
edgelet deprovision
```

After provisioning, EdgeletAPI requires signed JWTs (bootstrap unsigned tokens are rejected).

---

## Uninstall

```bash
sudo sh uninstall.sh               # remove binary and service, keep data
sudo sh uninstall.sh --remove-data # also remove /etc/edgelet, /var/lib/edgelet, …
```

See [troubleshooting.md](troubleshooting.md) if the service fails to stop cleanly.
