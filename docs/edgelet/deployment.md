# Edgelet deployment

This guide covers production installation of Edgelet on Linux edge nodes.

## Prerequisites

- Linux (Ubuntu 20.04+, RHEL/CentOS 8+, Debian 11+)
- Root or sudo access
- Network connectivity to the ioFog Controller
- For **lite** flavor: Docker 26.10+ or Podman 3.0+
- For **full** flavor: no external container runtime required

## Installation

Edgelet ships as **tarball + `install.sh` only** (no DEB/RPM in this product line).

```bash
curl -fsSL https://raw.githubusercontent.com/datasance/agent/main/install.sh | sudo sh -s -- --flavor=full
# or with a local tarball:
sudo sh install.sh --flavor=full --tarball-path=edgelet-linux-amd64-full.tar.gz
```

Tarball names: `edgelet-<VERSION>-linux-<ARCH>-{full|lite}.tar.gz`

| Flag | Purpose |
|------|---------|
| `--flavor=full\|lite` | Must match tarball (default: **full**) |
| `--container-engine=docker\|podman` | **lite** only; **full** always uses `edgelet` |
| `--airgap` | Offline install; requires `--tarball-path` |
| `--upgrade` / `--rollback` | In-place same-flavor OTA |

Supported init systems: **systemd**, OpenRC, SysV init, s6, runit, upstart.

Packaging details: [../../packaging/PACKAGING-STRUCTURE.md](../../packaging/PACKAGING-STRUCTURE.md).

---

## Daemon and systemd

Production **full** flavor uses the **thin** binary as the systemd entry point. The unit never invokes the fat runtime path directly.

```bash
edgelet daemon          # thin: extract if needed → exec fat … daemon
systemctl start edgelet # ExecStart=/usr/local/bin/edgelet daemon
```

Unit file: `packaging/systemd/edgelet.service` — `ExecStart=/usr/local/bin/edgelet daemon`.

On first start (or after upgrading the thin binary), `edgelet daemon` **lazy-extracts** the embedded zstd bundle into `/var/lib/edgelet/data/<sha256-prefix>/`, verifies contents, and updates `data/current` (retaining `data/previous` for the prior hash). Subsequent starts skip re-extract when `current` already points at a ready bundle.

**Operator CLI** (`edgelet ms`, `edgelet deploy`, `edgelet version`, …) runs in the **thin** process and does **not** trigger extract.

**Break-glass:** Run the fat runtime directly when debugging extract or dispatch issues:

```bash
/var/lib/edgelet/data/current/bin/edgelet daemon
```

**Lite** flavor: one monolithic `/usr/local/bin/edgelet` — CLI and daemon in the same ELF; no data-dir extract.

---

## Full vs lite flavor

Set at **compile time** via `internal/buildmeta`. The running config must match the binary.

| Flavor | `containerEngine` | Binary layout | Notes |
|--------|-------------------|---------------|--------|
| **full** | `edgelet` only | Thin OTA binary + fat runtime in embed tar | Thin `CGO=0` ≤ 55 MiB gate; fat in `/var/lib/edgelet/data/current/bin/edgelet`; linux only |
| **lite** | `docker` or `podman` | Monolithic ELF | `CGO=0`; linux, darwin, windows |

Verify flavor:

```bash
edgelet --version
edgelet system version -o json | jq .daemon.flavor
```

Changing **lite ↔ full** on the same host is not supported; uninstall and reinstall the correct flavor.

### OTA upgrade (full, thin binary)

Fleet OTA for **full** typically replaces only `/usr/local/bin/edgelet` (thin) and restarts `edgelet.service`. The new thin embed carries a new bundle hash; the first `edgelet daemon` after restart extracts into `/var/lib/edgelet/data/<new-hash>/` and rotates `current` / `previous`. Prior extracted trees may remain on disk until pruned manually.

`install.sh` tarball layout is unchanged in Plan 6; see packaging docs for install/upgrade flags. Greenfield reinstall is required when migrating from pre–Plan 6 single-ELF full layouts.

---

## Configuration

Primary config: `/etc/edgelet/config.yaml`

```yaml
controller: https://controller.example.com
deviceName: edge-device-01
containerEngine: edgelet          # full flavor
dockerUrl: unix:///run/edgelet/containerd.sock
logLevel: INFO
logDiskDirectory: /var/log/edgelet
profiles:
  production:
    containerEngine: edgelet
    dockerUrl: unix:///run/edgelet/containerd.sock
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
