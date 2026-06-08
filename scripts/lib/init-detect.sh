# init-detect.sh — detect linux init system (sourced by install.sh / uninstall.sh)

# OpenRC ships /sbin/openrc-run on Alpine even when PID 1 is busybox (Lima
# template:alpine). Require a running OpenRC supervisor, not merely openrc-run.
openrc_is_pid1() {
    [ -x /sbin/openrc-run ] || return 1
    if rc-status -s >/dev/null 2>&1; then
        return 0
    fi
    if [ -f /etc/inittab ] && grep -q '/sbin/openrc' /etc/inittab 2>/dev/null; then
        return 0
    fi
    _init="$(readlink -f /sbin/init 2>/dev/null || readlink /sbin/init 2>/dev/null || true)"
    case "${_init}" in
        *openrc*) return 0 ;;
    esac
    return 1
}

procd_is_openwrt() {
    [ -x /sbin/procd ] && [ -f /etc/rc.common ] || return 1
    return 0
}

detect_init() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /etc/systemd/system ]; then
        echo "systemd"
        return 0
    fi
    if procd_is_openwrt; then
        echo "procd"
        return 0
    fi
    if openrc_is_pid1 && {
        command -v openrc >/dev/null 2>&1 \
            || [ -x /sbin/openrc-run ] \
            || [ -f /sbin/openrc ]
    }; then
        echo "openrc"
        return 0
    fi
    if command -v initctl >/dev/null 2>&1 && [ -d /etc/init ]; then
        echo "upstart"
        return 0
    fi
    if [ -d /etc/s6 ] || command -v s6-svc >/dev/null 2>&1; then
        echo "s6"
        return 0
    fi
    if command -v runsvdir >/dev/null 2>&1 || [ -d /etc/runit ]; then
        echo "runit"
        return 0
    fi
    if [ -f /etc/inittab ] || command -v update-rc.d >/dev/null 2>&1 || command -v chkconfig >/dev/null 2>&1; then
        echo "sysvinit"
        return 0
    fi
    echo "unknown"
}
