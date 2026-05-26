#!/bin/sh
# uninstall.sh — Edgelet uninstaller
#
# Usage:
#   sudo sh uninstall.sh [--remove-data]
#
# --remove-data  also removes /var/lib/edgelet, /var/lib/edgelet-containerd,
#               /run/edgelet, /var/log/edgelet, and /etc/edgelet.

set -e

die() { echo "ERROR: $1" >&2; exit 1; }
info() { echo ">>> $1"; }

[ "$(id -u)" -eq 0 ] || die "Must be run as root. Try: sudo $0 $*"

REMOVE_DATA=false
for arg in "$@"; do
    case "${arg}" in
        --remove-data) REMOVE_DATA=true ;;
        --help)
            echo "Usage: $0 [--remove-data]"
            echo ""
            echo "  --remove-data   also delete all agent data directories"
            exit 0 ;;
    esac
done

# ── stop and disable service ──────────────────────────────────────────────────
stop_systemd() {
    if systemctl is-active --quiet edgelet 2>/dev/null; then
        systemctl stop edgelet
    fi
    systemctl disable edgelet 2>/dev/null || true
    rm -f /etc/systemd/system/edgelet.service
    systemctl daemon-reload 2>/dev/null || true
    info "systemd service removed."
}

stop_openrc() {
    rc-service edgelet stop 2>/dev/null || true
    rc-update del edgelet default 2>/dev/null || true
    rm -f /etc/init.d/edgelet
    info "OpenRC service removed."
}

stop_sysvinit() {
    /etc/init.d/edgelet stop 2>/dev/null || true
    update-rc.d -f edgelet remove 2>/dev/null || true
    rm -f /etc/init.d/edgelet
    info "SysV init service removed."
}

stop_upstart() {
    initctl stop edgelet 2>/dev/null || true
    rm -f /etc/init/edgelet.conf
    initctl reload-configuration 2>/dev/null || true
    info "Upstart service removed."
}

stop_s6() {
    s6-svc -d /var/run/s6/services/edgelet 2>/dev/null || true
    rm -f /var/run/s6/services/edgelet
    rm -rf /etc/s6/edgelet
    info "s6 service removed."
}

stop_runit() {
    rm -f /var/service/edgelet /service/edgelet 2>/dev/null || true
    rm -rf /etc/runit/edgelet
    info "runit service removed."
}

stop_fallback() {
    pkill -f "edgelet daemon" 2>/dev/null || true
    info "Background process stopped (if running)."
}

if command -v systemctl >/dev/null 2>&1 && [ -f /etc/systemd/system/edgelet.service ]; then
    stop_systemd
elif command -v openrc >/dev/null 2>&1 && [ -f /etc/init.d/edgelet ]; then
    stop_openrc
elif [ -f /etc/init/edgelet.conf ]; then
    stop_upstart
elif [ -f /etc/init.d/edgelet ]; then
    stop_sysvinit
elif [ -d /etc/s6/edgelet ]; then
    stop_s6
elif [ -d /etc/runit/edgelet ]; then
    stop_runit
else
    stop_fallback
fi

# ── remove binaries ───────────────────────────────────────────────────────────
rm -f /usr/local/bin/edgelet
info "Binary removed."

# ── optionally remove data ────────────────────────────────────────────────────
if [ "${REMOVE_DATA}" = "true" ]; then
    info "Removing agent data directories..."

    if command -v umount >/dev/null 2>&1; then
        mount | grep '/run/edgelet\|/var/lib/edgelet' | awk '{print $3}' | \
            sort -r | while read -r mp; do
            umount -l "${mp}" 2>/dev/null || true
        done
    fi

    rm -rf /var/lib/edgelet
    rm -rf /var/lib/edgelet-containerd
    rm -rf /run/edgelet
    rm -rf /var/log/edgelet
    info "Agent data directories removed."
else
    info "Data directories preserved (use --remove-data to remove):"
    info "  /var/lib/edgelet"
    info "  /var/lib/edgelet-containerd"
    info "  /run/edgelet"
    info "  /var/log/edgelet"
fi

# ── optional: remove config ───────────────────────────────────────────────────
if [ "${REMOVE_DATA}" = "true" ]; then
    rm -rf /etc/edgelet
    info "Configuration directory /etc/edgelet removed."
fi

info ""
info "Edgelet has been uninstalled."
