#!/bin/sh
# uninstall.sh — Edgelet uninstaller
#
# Usage:
#   sudo sh uninstall.sh [--remove-data]
#
# --remove-data  also removes /var/lib/edgelet, /var/lib/edgelet-containerd,
#               /run/edgelet, /var/log/edgelet, /etc/edgelet, /var/backups/edgelet

set -e

die() { echo "ERROR: $1" >&2; exit 1; }
info() { echo ">>> $1"; }

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
if [ -f "${SCRIPT_DIR}/scripts/lib/init-detect.sh" ]; then
    LIB_DIR="${SCRIPT_DIR}/scripts/lib"
elif [ -f "${SCRIPT_DIR}/lib/init-detect.sh" ]; then
    LIB_DIR="${SCRIPT_DIR}/lib"
else
    LIB_DIR=""
fi
if [ -n "$LIB_DIR" ] && [ -f "${LIB_DIR}/init-detect.sh" ]; then
    # shellcheck source=scripts/lib/init-detect.sh
    . "${LIB_DIR}/init-detect.sh"
    # shellcheck source=scripts/lib/init-edgelet.sh
    . "${LIB_DIR}/init-edgelet.sh"
fi

[ "$(id -u)" -eq 0 ] || die "Must be run as root. Try: sudo $0 $*"

REMOVE_DATA=false
for arg in "$@"; do
    case "${arg}" in
        --remove-data) REMOVE_DATA=true ;;
        --help|-h)
            echo "Usage: $0 [--remove-data]"
            echo ""
            echo "  --remove-data   also delete config, data, and backup directories"
            exit 0 ;;
        *) die "Unknown option: ${arg}" ;;
    esac
done

stop_systemd() {
    if systemctl is-active --quiet edgelet 2>/dev/null; then
        systemctl stop edgelet
    fi
    systemctl disable edgelet 2>/dev/null || true
    rm -f /etc/systemd/system/edgelet.service
    rm -f /etc/systemd/system/edgelet-containerd.service
    rm -rf /etc/systemd/system/edgelet.service.d
    systemctl daemon-reload 2>/dev/null || true
    info "systemd service removed."
}

stop_procd() {
    /etc/init.d/edgelet stop 2>/dev/null || true
    /etc/init.d/edgelet disable 2>/dev/null || true
    rm -f /etc/init.d/edgelet
    info "procd init script removed."
}

stop_openrc() {
    rc-service edgelet stop 2>/dev/null || true
    rc-update del edgelet default 2>/dev/null || true
    rm -f /etc/init.d/edgelet /etc/init.d/edgelet-containerd
    info "OpenRC service removed."
}

stop_sysvinit() {
    /etc/init.d/edgelet stop 2>/dev/null || true
    update-rc.d -f edgelet remove 2>/dev/null || true
    chkconfig --del edgelet 2>/dev/null || true
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
    rm -rf /var/run/s6/services/edgelet
    rm -rf /etc/s6/edgelet
    info "s6 service removed."
}

stop_runit() {
    sv down edgelet 2>/dev/null || true
    rm -f /etc/service/edgelet /var/service/edgelet /service/edgelet 2>/dev/null || true
    rm -rf /etc/runit/edgelet
    info "runit service removed."
}

stop_fallback() {
    pkill -f "/usr/local/bin/edgelet daemon" 2>/dev/null || true
    pkill -f "edgelet daemon" 2>/dev/null || true
    info "Background edgelet processes stopped (if any)."
}

lazy_umount_edgelet() {
    if ! command -v umount >/dev/null 2>&1; then
        return 0
    fi
    mount 2>/dev/null | grep -E '/run/edgelet|/var/lib/edgelet' | awk '{print $3}' | \
        sort -r | while read -r mp; do
            [ -n "$mp" ] || continue
            umount -l "${mp}" 2>/dev/null || true
        done
}

remove_init_service() {
    _init=""
    if [ -n "$LIB_DIR" ]; then
        _init=$(detect_init 2>/dev/null) || true
    fi
    if [ -f /etc/systemd/system/edgelet.service ]; then
        stop_systemd
    elif [ -f /etc/init.d/edgelet ] && [ -x /sbin/procd ]; then
        stop_procd
    elif [ -f /etc/init.d/edgelet ] && command -v openrc >/dev/null 2>&1; then
        stop_openrc
    elif [ -f /etc/init/edgelet.conf ]; then
        stop_upstart
    elif [ -f /etc/init.d/edgelet ]; then
        stop_sysvinit
    elif [ -d /etc/s6/edgelet ]; then
        stop_s6
    elif [ -d /etc/runit/edgelet ] || [ -L /var/service/edgelet ]; then
        stop_runit
    elif [ "$_init" != "" ] && [ "$_init" != "unknown" ]; then
        case "$_init" in
            systemd) stop_systemd ;;
            openrc) stop_openrc ;;
            procd) stop_procd ;;
            sysvinit) stop_sysvinit ;;
            upstart) stop_upstart ;;
            s6) stop_s6 ;;
            runit) stop_runit ;;
            *) stop_fallback ;;
        esac
    else
        stop_fallback
    fi
}

remove_init_service
lazy_umount_edgelet

rm -f /usr/local/bin/edgelet
info "Binary removed."

rm -rf /usr/libexec/edgelet
info "Init helpers removed from /usr/libexec/edgelet/"

rm -rf /usr/share/edgelet
info "Bundled scripts removed from /usr/share/edgelet/"

if [ "${REMOVE_DATA}" = "true" ]; then
    info "Removing agent data directories..."
    lazy_umount_edgelet
    rm -rf /var/lib/edgelet
    rm -rf /var/lib/edgelet-containerd
    rm -rf /run/edgelet
    rm -rf /var/log/edgelet
    rm -rf /var/backups/edgelet
    rm -rf /etc/edgelet
    info "Data, backups, and configuration removed."
else
    info "Data directories preserved (use --remove-data to remove):"
    info "  /var/lib/edgelet"
    info "  /var/lib/edgelet-containerd"
    info "  /run/edgelet"
    info "  /var/log/edgelet"
    info "  /var/backups/edgelet"
    info "  /etc/edgelet"
fi

info ""
info "Edgelet has been uninstalled."
