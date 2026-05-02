#!/usr/bin/env bash
# test/embedded/vm-setup-inside.sh
#
# This script runs INSIDE the Lima VM (as root via sudo).
# It is executed by vm-install.sh via a single limactl shell call,
# avoiding all SSH quoting/escaping issues with complex commands.
#
# Arguments (positional):
#   $1  DAEMON_BIN   — path to iofog-agentd binary (same path as on Mac via Lima mount)
#   $2  CLI_BIN      — path to iofog-agent binary
#   $3  CONFIG_SRC   — path to config_new.yaml
#   $4  CERT_SRC     — path to cert_new.crt
#
# Do NOT call this script directly; use vm-install.sh.

set -euo pipefail

DAEMON_BIN="$1"
CLI_BIN="$2"
CONFIG_SRC="$3"


echo "[vm-setup] Stopping any existing iofog-agentd..."
# Use full binary path to avoid pkill self-match.
# Avoid 'systemctl stop' on a non-existent service (waits 45s on Ubuntu 24.04).
if systemctl is-active --quiet iofog-agentd 2>/dev/null; then
    systemctl stop iofog-agentd 2>/dev/null || true
fi
pkill -f '/usr/local/bin/iofog-agentd' 2>/dev/null || true
sleep 1

echo "[vm-setup] Copying binaries..."
cp "${DAEMON_BIN}" /usr/local/bin/iofog-agentd
chmod 755 /usr/local/bin/iofog-agentd
cp "${CLI_BIN}" /usr/local/bin/iofog-agent
chmod 755 /usr/local/bin/iofog-agent
file /usr/local/bin/iofog-agentd

echo "[vm-setup] Creating directories..."
mkdir -p /etc/iofog-agent
mkdir -p /var/lib/iofog-agent /var/log/iofog-agent /run/iofog-agent
mkdir -p /var/lib/iofog-agent-containerd
mkdir -p /var/log/iofog-microservices
chmod 750 /etc/iofog-agent /var/lib/iofog-agent /var/log/iofog-agent /run/iofog-agent /var/log/iofog-microservices /var/lib/iofog-agent-containerd

echo "[vm-setup] Writing config..."
cp "${CONFIG_SRC}" /etc/iofog-agent/config.yaml
sed -i 's/containerEngine: .*/containerEngine: "iofog"/' /etc/iofog-agent/config.yaml
echo "  containerEngine = iofog set"

echo "[vm-setup] Generating local-api token..."
head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c 32 > /etc/iofog-agent/local-api

echo "[vm-setup] Installing systemd service..."
cat > /etc/systemd/system/iofog-agentd.service << 'UNIT'
[Unit]
Description=ioFog Agent Daemon (embedded containerd)
Documentation=https://docs.datasance.com
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=10

[Service]
Type=simple
ExecStart=/usr/local/bin/iofog-agentd start
Restart=always
RestartSec=3s
StandardOutput=journal
StandardError=journal
SyslogIdentifier=iofog-agentd

# Embedded containerd needs full root capabilities:
# CAP_CHOWN/CAP_FOWNER for overlayfs layer extraction (lchown file ownership)
# CAP_SYS_ADMIN for mount(), overlay filesystems, namespaces
# CAP_NET_ADMIN for CNI bridge network creation
# CAP_MKNOD for device nodes inside containers
# Removing CapabilityBoundingSet restriction — a limited bounding set prevents
# even root from using capabilities not listed, breaking overlayfs extraction.
NoNewPrivileges=no

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable iofog-agentd

echo "[vm-setup] Starting iofog-agentd..."
systemctl start iofog-agentd

echo "[vm-setup] Waiting for containerd socket (up to 60s)..."
elapsed=0
while [[ ! -S /run/iofog-agent/containerd.sock ]]; do
    if (( elapsed >= 60 )); then
        echo "[vm-setup] ERROR: containerd socket not ready after 60s. Daemon logs:"
        journalctl -u iofog-agentd -n 40 --no-pager || true
        exit 1
    fi
    echo -n "."
    sleep 2
    (( elapsed += 2 ))
done
echo ""
echo "[vm-setup] containerd socket is ready."

###############################################################################
# Install matching ctr binary (v2.1.5) so test scripts can talk to our
# embedded containerd without version-mismatch "unknown service streaming" errors.
###############################################################################
CONTAINERD_VER="2.1.5"
ARCH="$(uname -m)"
[[ "${ARCH}" == "aarch64" ]] && CTR_ARCH="arm64" || CTR_ARCH="amd64"
CTR_URL="https://github.com/containerd/containerd/releases/download/v${CONTAINERD_VER}/containerd-${CONTAINERD_VER}-linux-${CTR_ARCH}.tar.gz"

if ctr --version 2>/dev/null | grep -q "${CONTAINERD_VER}"; then
    echo "[vm-setup] ctr ${CONTAINERD_VER} already installed."
else
    echo "[vm-setup] Installing ctr ${CONTAINERD_VER} (matching embedded containerd)..."
    TMP_CTR="$(mktemp -d)"
    curl -fsSL "${CTR_URL}" | tar -xzf - -C "${TMP_CTR}" bin/ctr
    mv "${TMP_CTR}/bin/ctr" /usr/local/bin/ctr-iofog
    chmod 755 /usr/local/bin/ctr-iofog
    rm -rf "${TMP_CTR}"
    # Symlink as 'ctr' only if no same-version system ctr exists
    ln -sf /usr/local/bin/ctr-iofog /usr/local/bin/ctr
    echo "[vm-setup] ctr installed at /usr/local/bin/ctr -> ctr-iofog (${CONTAINERD_VER})"
fi

echo "[vm-setup] Done."
