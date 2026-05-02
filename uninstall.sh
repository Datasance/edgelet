#!/bin/sh
# uninstall.sh — iofog-agent uninstaller
#
# Usage:
#   sudo sh uninstall.sh [--remove-data]
#
# --remove-data  also removes /var/lib/iofog-agent, /var/lib/iofog-agent-containerd,
#               /run/iofog-agent, and /var/log/iofog-agent.

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
    if systemctl is-active --quiet iofog-agentd 2>/dev/null; then
        systemctl stop iofog-agentd
    fi
    systemctl disable iofog-agentd 2>/dev/null || true
    rm -f /etc/systemd/system/iofog-agentd.service
    systemctl daemon-reload 2>/dev/null || true
    info "systemd service removed."
}

stop_openrc() {
    rc-service iofog-agentd stop 2>/dev/null || true
    rc-update del iofog-agentd default 2>/dev/null || true
    rm -f /etc/init.d/iofog-agentd
    info "OpenRC service removed."
}

stop_sysvinit() {
    /etc/init.d/iofog-agentd stop 2>/dev/null || true
    update-rc.d -f iofog-agentd remove 2>/dev/null || true
    rm -f /etc/init.d/iofog-agentd
    info "SysV init service removed."
}

stop_upstart() {
    initctl stop iofog-agentd 2>/dev/null || true
    rm -f /etc/init/iofog-agentd.conf
    initctl reload-configuration 2>/dev/null || true
    info "Upstart service removed."
}

stop_s6() {
    s6-svc -d /var/run/s6/services/iofog-agentd 2>/dev/null || true
    rm -f /var/run/s6/services/iofog-agentd
    rm -rf /etc/s6/iofog-agentd
    info "s6 service removed."
}

stop_runit() {
    rm -f /var/service/iofog-agentd /service/iofog-agentd 2>/dev/null || true
    rm -rf /etc/runit/iofog-agentd
    info "runit service removed."
}

stop_fallback() {
    pkill -f "iofog-agentd daemon" 2>/dev/null || true
    info "Background process stopped (if running)."
}

if command -v systemctl >/dev/null 2>&1 && [ -f /etc/systemd/system/iofog-agentd.service ]; then
    stop_systemd
elif command -v openrc >/dev/null 2>&1 && [ -f /etc/init.d/iofog-agentd ]; then
    stop_openrc
elif [ -f /etc/init/iofog-agentd.conf ]; then
    stop_upstart
elif [ -f /etc/init.d/iofog-agentd ]; then
    stop_sysvinit
elif [ -d /etc/s6/iofog-agentd ]; then
    stop_s6
elif [ -d /etc/runit/iofog-agentd ]; then
    stop_runit
else
    stop_fallback
fi

# ── remove binaries ───────────────────────────────────────────────────────────
rm -f /usr/local/bin/iofog-agent /usr/local/bin/iofog-agentd
info "Binaries removed."

# ── optionally remove data ────────────────────────────────────────────────────
if [ "${REMOVE_DATA}" = "true" ]; then
    info "Removing agent data directories..."

    # Unmount any containerd overlay mounts before removing data.
    # Containerd creates overlayfs mounts under its state directory; if we just
    # rm -rf them while they're mounted the operation silently leaves orphan mounts.
    if command -v umount >/dev/null 2>&1; then
        # Find and unmount any overlay mounts under iofog-agent paths.
        mount | grep '/run/iofog-agent\|/var/lib/iofog-agent' | awk '{print $3}' | \
            sort -r | while read -r mp; do
            umount -l "${mp}" 2>/dev/null || true
        done
    fi

    rm -rf /var/lib/iofog-agent
    rm -rf /var/lib/iofog-agent-containerd
    rm -rf /run/iofog-agent
    rm -rf /var/log/iofog-agent
    info "Agent data directories removed."
else
    info "Data directories preserved (use --remove-data to remove):"
    info "  /var/lib/iofog-agent"
    info "  /var/lib/iofog-agent-containerd"
    info "  /run/iofog-agent"
    info "  /var/log/iofog-agent"
fi

# ── optional: remove config ───────────────────────────────────────────────────
if [ "${REMOVE_DATA}" = "true" ]; then
    rm -rf /etc/iofog-agent
    info "Configuration directory /etc/iofog-agent removed."
fi

info ""
info "iofog-agent has been uninstalled."
