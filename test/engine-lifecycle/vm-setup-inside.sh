#!/usr/bin/env bash
# Runs inside the Lima VM as root.
set -euo pipefail

EDGELET_BIN="$1"
CONFIG_SRC="$2"
CERT_SRC="$3"
START_ENGINE="${4:-edgelet}"

echo "[vm-setup] start engine: ${START_ENGINE}"
systemctl stop edgelet 2>/dev/null || true
pkill -f '/usr/local/bin/edgelet' 2>/dev/null || true
sleep 1
rm -f /var/lib/edgelet/edgelet.db /var/lib/edgelet/edgelet.db-wal /var/lib/edgelet/edgelet.db-shm

cp "${EDGELET_BIN}" /usr/local/bin/edgelet
chmod 755 /usr/local/bin/edgelet
mkdir -p /etc/edgelet /var/lib/edgelet /var/log/edgelet /run/edgelet /var/lib/edgelet-containerd
chmod 750 /etc/edgelet /var/lib/edgelet /var/log/edgelet /run/edgelet /var/lib/edgelet-containerd

cp "${CONFIG_SRC}" /etc/edgelet/config.yaml
cp "${CERT_SRC}" /etc/edgelet/cert.crt
chmod 640 /etc/edgelet/cert.crt

sed -i "s/containerEngine: \"edgelet\"/containerEngine: \"${START_ENGINE}\"/" /etc/edgelet/config.yaml
sed -i "s/containerEngine: edgelet/containerEngine: ${START_ENGINE}/" /etc/edgelet/config.yaml
if [[ "${START_ENGINE}" == "edgelet" ]]; then
    sed -i 's|dockerUrl: "unix:///var/run/docker.sock"|dockerUrl: "unix:///run/edgelet/containerd.sock"|' /etc/edgelet/config.yaml
    sed -i 's|dockerUrl: unix:///var/run/docker.sock|dockerUrl: unix:///run/edgelet/containerd.sock|' /etc/edgelet/config.yaml
else
    sed -i 's|dockerUrl: "unix:///run/edgelet/containerd.sock"|dockerUrl: "unix:///var/run/docker.sock"|' /etc/edgelet/config.yaml
    sed -i 's|dockerUrl: unix:///run/edgelet/containerd.sock|dockerUrl: unix:///var/run/docker.sock|' /etc/edgelet/config.yaml
fi

cat > /etc/systemd/system/edgelet.service << 'UNIT'
[Unit]
Description=Edgelet daemon
After=network-online.target docker.service
Wants=network-online.target
StartLimitIntervalSec=300
StartLimitBurst=20

[Service]
Type=simple
ExecStart=/usr/local/bin/edgelet daemon
Restart=always
RestartSec=2s
TimeoutStopSec=120s
KillMode=process
Delegate=yes
StandardOutput=journal
StandardError=journal
SyslogIdentifier=edgelet
NoNewPrivileges=no

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable edgelet
systemctl restart edgelet

for i in $(seq 1 60); do
    if edgelet system status 2>/dev/null | grep -q runtime.engineReady; then
        break
    fi
    sleep 2
done
echo "[vm-setup] edgelet started with containerEngine=${START_ENGINE}"
