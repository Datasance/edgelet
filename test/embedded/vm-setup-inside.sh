#!/usr/bin/env bash
# test/embedded/vm-setup-inside.sh
#
# This script runs INSIDE the Lima VM (as root via sudo).
# It is executed by vm-install.sh via a single limactl shell call,
# avoiding all SSH quoting/escaping issues with complex commands.
#
# Arguments (positional):
#   $1  EDGELET_BIN  — path to edgelet binary (staged via scp)
#   $2  CONFIG_SRC   — path to config_new.yaml
#   $3  CERT_SRC     — path to cert_new.crt
#
# Do NOT call this script directly; use vm-install.sh.

set -euo pipefail

EDGELET_BIN="$1"
CONFIG_SRC="$2"
CERT_SRC="$3"

echo "[vm-setup] Stopping any existing edgelet..."
if systemctl is-active --quiet edgelet 2>/dev/null; then
    systemctl stop edgelet 2>/dev/null || true
fi
pkill -f '/usr/local/bin/edgelet' 2>/dev/null || true
sleep 1

echo "[vm-setup] Copying edgelet binary..."
cp "${EDGELET_BIN}" /usr/local/bin/edgelet
chmod 755 /usr/local/bin/edgelet
ls -l /usr/local/bin/edgelet

echo "[vm-setup] Creating directories..."
mkdir -p /etc/edgelet
mkdir -p /var/lib/edgelet /var/log/edgelet /run/edgelet
mkdir -p /var/lib/edgelet-containerd
chmod 750 /etc/edgelet /var/lib/edgelet /var/log/edgelet /run/edgelet /var/lib/edgelet-containerd

echo "[vm-setup] Writing config..."
cp "${CONFIG_SRC}" /etc/edgelet/config.yaml
cp "${CERT_SRC}" /etc/edgelet/cert.crt
chmod 640 /etc/edgelet/cert.crt
echo "  containerEngine = edgelet (from config template)"

echo "[vm-setup] Installing systemd service..."
cat > /etc/systemd/system/edgelet.service << 'UNIT'
[Unit]
Description=Edgelet daemon (embedded containerd)
Documentation=https://github.com/datasance/edgelet
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=20

[Service]
Type=simple
ExecStart=/usr/local/bin/edgelet daemon
Restart=always
RestartSec=2s
TimeoutStopSec=120s
KillMode=control-group
KillSignal=SIGTERM
SendSIGKILL=yes
StandardOutput=journal
StandardError=journal
SyslogIdentifier=edgelet

# Embedded containerd needs full root capabilities:
# CAP_CHOWN/CAP_FOWNER for overlayfs layer extraction (lchown file ownership)
# CAP_SYS_ADMIN for mount(), overlay filesystems, namespaces
# CAP_NET_ADMIN for CNI bridge network creation
# CAP_MKNOD for device nodes inside containers
NoNewPrivileges=no

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable edgelet

echo "[vm-setup] Starting edgelet..."
systemctl start edgelet

echo "[vm-setup] Waiting for containerd socket (up to 60s)..."
elapsed=0
while [[ ! -S /run/edgelet/containerd.sock ]]; do
    if (( elapsed >= 60 )); then
        echo "[vm-setup] ERROR: containerd socket not ready after 60s. Daemon logs:"
        journalctl -u edgelet -n 40 --no-pager || true
        exit 1
    fi
    echo -n "."
    sleep 2
    (( elapsed += 2 ))
done
echo ""
echo "[vm-setup] containerd socket is ready."
echo "[vm-setup] Done."
