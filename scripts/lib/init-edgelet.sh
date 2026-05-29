# init-edgelet.sh — install linux init unit for edgelet (sourced by install.sh)

init_packaging_root() {
    if [ -n "${EDGELET_INIT_DIR}" ] && [ -d "${EDGELET_INIT_DIR}" ]; then
        echo "${EDGELET_INIT_DIR}"
        return 0
    fi
    if [ -d "${SCRIPT_DIR}/packaging/init" ]; then
        echo "${SCRIPT_DIR}/packaging/init"
        return 0
    fi
    if [ -d /usr/share/edgelet/init ]; then
        echo /usr/share/edgelet/init
        return 0
    fi
    die "Init templates not found (packaging/init or /usr/share/edgelet/init)"
}

install_init_unit() {
    _init="$1"
    _eng="$2"
    _root="$(init_packaging_root)"
    mkdir -p /var/log/edgelet

    case "${_init}" in
        systemd)
            _unit="${_root}/systemd/edgelet.service"
            [ -f "$_unit" ] || die "Missing ${_unit}"
            install -m 644 "$_unit" /etc/systemd/system/edgelet.service
            case "${_eng}" in
                docker)
                    sed -i '/^After=/s/.*/After=network-online.target docker.service/' /etc/systemd/system/edgelet.service 2>/dev/null || true
                    grep -q 'Wants=docker.service' /etc/systemd/system/edgelet.service 2>/dev/null || \
                        sed -i '/^After=/a Wants=docker.service' /etc/systemd/system/edgelet.service 2>/dev/null || true
                    ;;
                podman)
                    sed -i '/^After=/s/.*/After=network-online.target podman.socket/' /etc/systemd/system/edgelet.service 2>/dev/null || true
                    ;;
            esac
            systemctl daemon-reload
            systemctl enable edgelet
            systemctl restart edgelet
            info "systemd unit edgelet.service installed and started."
            ;;
        openrc)
            install -m 755 "${_root}/openrc/edgelet.init" /etc/init.d/edgelet
            rc-update add edgelet default 2>/dev/null || true
            rc-service edgelet restart 2>/dev/null || rc-service edgelet start
            info "OpenRC service edgelet installed."
            ;;
        sysvinit)
            install -m 755 "${_root}/sysvinit/edgelet.init" /etc/init.d/edgelet
            if command -v update-rc.d >/dev/null 2>&1; then
                update-rc.d edgelet defaults 2>/dev/null || true
            elif command -v chkconfig >/dev/null 2>&1; then
                chkconfig --add edgelet 2>/dev/null || true
            fi
            /etc/init.d/edgelet restart 2>/dev/null || /etc/init.d/edgelet start
            info "SysV init script edgelet installed."
            ;;
        upstart)
            install -m 644 "${_root}/upstart/edgelet.conf" /etc/init/edgelet.conf
            initctl reload-configuration 2>/dev/null || true
            initctl restart edgelet 2>/dev/null || initctl start edgelet
            info "Upstart job edgelet installed."
            ;;
        s6)
            mkdir -p /etc/s6/edgelet
            install -m 755 "${_root}/s6/run" /etc/s6/edgelet/run
            install -m 755 "${_root}/s6/finish" /etc/s6/edgelet/finish
            if command -v s6-svc >/dev/null 2>&1 && [ -d /var/run/s6/services ]; then
                mkdir -p /var/run/s6/services/edgelet
                ln -sf /etc/s6/edgelet /var/run/s6/services/edgelet/supervise 2>/dev/null || true
                s6-svc -u /var/run/s6/services/edgelet 2>/dev/null || true
            fi
            info "s6 service installed under /etc/s6/edgelet (start via your s6 scan)."
            ;;
        runit)
            mkdir -p /etc/runit/edgelet
            install -m 755 "${_root}/runit/run" /etc/runit/edgelet/run
            if [ -d /etc/runit ]; then
                ln -sf /etc/runit/edgelet /etc/service/edgelet 2>/dev/null || \
                    ln -sf /etc/runit/edgelet /var/service/edgelet 2>/dev/null || true
                sv restart edgelet 2>/dev/null || sv start edgelet 2>/dev/null || true
            fi
            info "runit service installed under /etc/runit/edgelet."
            ;;
        *)
            die "No supported init system detected (${_init}). Install systemd, openrc, sysvinit, upstart, s6, or runit."
            ;;
    esac
}

stop_edgelet_service() {
    _init="${1:-$(detect_init)}"
    case "${_init}" in
        systemd) systemctl stop edgelet 2>/dev/null || true ;;
        openrc) rc-service edgelet stop 2>/dev/null || true ;;
        sysvinit) /etc/init.d/edgelet stop 2>/dev/null || true ;;
        upstart) initctl stop edgelet 2>/dev/null || true ;;
        s6) s6-svc -d /var/run/s6/services/edgelet 2>/dev/null || true ;;
        runit) sv down edgelet 2>/dev/null || true ;;
        *) pkill -f "/usr/local/bin/edgelet daemon" 2>/dev/null || true ;;
    esac
}
