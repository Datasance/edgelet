# init-detect.sh — detect linux init system (sourced by install.sh / uninstall.sh)

detect_init() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /etc/systemd/system ]; then
        echo "systemd"
        return 0
    fi
    if command -v openrc >/dev/null 2>&1 || [ -x /sbin/openrc-run ] || [ -f /sbin/openrc ]; then
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
