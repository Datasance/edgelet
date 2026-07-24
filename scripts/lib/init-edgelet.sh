# init-edgelet.sh — install linux init unit for edgelet (sourced by install.sh)

EDGELET_LIBEXEC="/usr/libexec/edgelet"
EDGELET_INIT_SHARE="/usr/share/edgelet/init"

init_packaging_root() {
    if [ -n "${EDGELET_INIT_DIR}" ] && [ -d "${EDGELET_INIT_DIR}" ]; then
        echo "${EDGELET_INIT_DIR}"
        return 0
    fi
    if [ -d "${SCRIPT_DIR}/packaging/init" ]; then
        echo "${SCRIPT_DIR}/packaging/init"
        return 0
    fi
    if [ -d "${EDGELET_INIT_SHARE}" ]; then
        echo "${EDGELET_INIT_SHARE}"
        return 0
    fi
    die "Init templates not found (packaging/init or ${EDGELET_INIT_SHARE})"
}

edgelet_shutdown_script() {
    if [ -f "${SCRIPT_DIR}/scripts/edgelet-shutdown" ]; then
        echo "${SCRIPT_DIR}/scripts/edgelet-shutdown"
        return 0
    fi
    if [ -f "${SHARE_DIR}/edgelet-shutdown" ]; then
        echo "${SHARE_DIR}/edgelet-shutdown"
        return 0
    fi
    die "Missing edgelet-shutdown helper (scripts/edgelet-shutdown)"
}

install_init_helpers() {
    _shutdown="$(edgelet_shutdown_script)"
    mkdir -p "${EDGELET_LIBEXEC}"
    install -m 755 "${_shutdown}" "${EDGELET_LIBEXEC}/edgelet-shutdown"
}

openrc_engine_need_line() {
    case "$1" in
        docker) printf '%s\n' '    need docker' ;;
        podman) printf '%s\n' '    need podman' ;;
        edgelet) printf '%s\n' '    need edgelet-containerd' ;;
        *)      printf '%s\n' '' ;;
    esac
}

apply_openrc_engine_deps() {
    _eng="$1"
    _dest="$2"
    _need="$(openrc_engine_need_line "${_eng}")"
    if [ -f "${_dest}" ]; then
        # shellcheck disable=SC2016
        awk -v need="${_need}" '
            /%%EDGELET_ENGINE_NEED%%/ {
                if (need != "") print need
                next
            }
            { print }
        ' "${_dest}" > "${_dest}.tmp" && mv "${_dest}.tmp" "${_dest}"
    fi
}

# installed_embed_hash returns the basename of data/current (embed SHA256 prefix), or empty.
installed_embed_hash() {
    _link="/var/lib/edgelet/data/current"
    [ -L "$_link" ] || return 0
    basename "$(readlink -f "$_link" 2>/dev/null || readlink "$_link")"
}

# binary_embed_hash reads embed hash from a linux thin binary (edgelet version --verbose).
binary_embed_hash() {
    _bin="$1"
    [ -x "$_bin" ] || return 0
    "$_bin" version --verbose 2>/dev/null \
        | sed -n 's/^  embed hash: //p' \
        | head -1
}

# should_restart_data_plane is true when containerEngine=edgelet and the embed hash changed
# (or data/current is not yet set). Returns false for docker/podman or lite builds without embed.
should_restart_data_plane() {
    _eng="$1"
    _old="$2"
    _new="$3"
    [ "$_eng" = "edgelet" ] || return 1
    [ -n "$_new" ] || return 1
    [ -z "$_old" ] && return 0
    [ "$_old" != "$_new" ]
}

start_edgelet_containerd_unit() {
    _init="$1"
    _restart="$2"
    case "${_init}" in
        systemd)
            systemctl enable edgelet-containerd 2>/dev/null || true
            if [ "$_restart" = true ]; then
                info "Embedded bundle hash changed; restarting edgelet-containerd (data plane)"
                systemctl stop edgelet-containerd 2>/dev/null || true
                systemctl reset-failed edgelet-containerd 2>/dev/null || true
                systemctl start edgelet-containerd
            else
                systemctl start edgelet-containerd 2>/dev/null || true
            fi
            ;;
        openrc)
            rc-update add edgelet-containerd default 2>/dev/null || true
            if [ "$_restart" = true ]; then
                info "Embedded bundle hash changed; restarting edgelet-containerd (data plane)"
                rc-service edgelet-containerd stop 2>/dev/null || true
                rc-service edgelet-containerd start 2>/dev/null || true
            else
                rc-service edgelet-containerd start 2>/dev/null || true
            fi
            ;;
    esac
}

install_systemd_dropin() {
    _eng="$1"
    _root="$2"
    _dropdir="/etc/systemd/system/edgelet.service.d"
    mkdir -p "${_dropdir}"
    rm -f "${_dropdir}/docker.conf" "${_dropdir}/podman.conf" "${_dropdir}/edgelet.conf"
    case "${_eng}" in
        docker)
            install -m 644 "${_root}/systemd/edgelet.service.d/docker.conf" "${_dropdir}/docker.conf"
            ;;
        podman)
            install -m 644 "${_root}/systemd/edgelet.service.d/podman.conf" "${_dropdir}/podman.conf"
            ;;
        edgelet)
            install -m 644 "${_root}/systemd/edgelet.service.d/edgelet.conf" "${_dropdir}/edgelet.conf"
            systemctl enable edgelet-containerd 2>/dev/null || true
            ;;
    esac
}

install_init_unit() {
    _init="$1"
    _eng="$2"
    _restart_dp="${3:-false}"
    _root="$(init_packaging_root)"
    mkdir -p /var/log/edgelet
    install_init_helpers

    case "${_init}" in
        systemd)
            _unit="${_root}/systemd/edgelet.service"
            [ -f "$_unit" ] || die "Missing ${_unit}"
            mkdir -p /etc/cni/net.d /run/edgelet /run/containerd
            chmod 755 /run/edgelet /run/containerd 2>/dev/null || true
            install -m 644 "$_unit" /etc/systemd/system/edgelet.service
            _containerd="${_root}/systemd/edgelet-containerd.service"
            if [ -f "${_containerd}" ]; then
                install -m 644 "${_containerd}" /etc/systemd/system/edgelet-containerd.service
            fi
            install_systemd_dropin "${_eng}" "${_root}"
            systemctl daemon-reload
            if [ "${_eng}" = "edgelet" ]; then
                start_edgelet_containerd_unit "${_init}" "${_restart_dp}"
            fi
            systemctl enable edgelet
            systemctl stop edgelet 2>/dev/null || true
            systemctl reset-failed edgelet 2>/dev/null || true
            systemctl start edgelet
            info "systemd unit edgelet.service installed (engine=${_eng} drop-in)."
            ;;
        openrc)
            install -m 755 "${_root}/openrc/edgelet.init" /etc/init.d/edgelet
            apply_openrc_engine_deps "${_eng}" /etc/init.d/edgelet
            chmod 755 /etc/init.d/edgelet
            if [ -f "${_root}/openrc/edgelet-cgroup-prep.init" ]; then
                install -m 755 "${_root}/openrc/edgelet-cgroup-prep.init" /etc/init.d/edgelet-cgroup-prep
                rc-update add edgelet-cgroup-prep sysinit 2>/dev/null || true
            fi
            if [ -f "${_root}/openrc/edgelet-containerd.init" ]; then
                install -m 755 "${_root}/openrc/edgelet-containerd.init" /etc/init.d/edgelet-containerd
            fi
            if [ "${_eng}" = "edgelet" ]; then
                start_edgelet_containerd_unit "${_init}" "${_restart_dp}"
            fi
            rc-update add edgelet default 2>/dev/null || true
            rc-service edgelet restart 2>/dev/null || rc-service edgelet start
            info "OpenRC service edgelet installed (engine=${_eng})."
            ;;
        procd)
            install -m 755 "${_root}/procd/edgelet" /etc/init.d/edgelet
            /etc/init.d/edgelet enable 2>/dev/null || true
            /etc/init.d/edgelet stop 2>/dev/null || true
            /etc/init.d/edgelet start
            info "procd init script edgelet installed (engine=${_eng})."
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
            if [ -f "${_root}/runit/finish" ]; then
                install -m 755 "${_root}/runit/finish" /etc/runit/edgelet/finish
            fi
            if [ -d /etc/runit ]; then
                ln -sf /etc/runit/edgelet /etc/service/edgelet 2>/dev/null || \
                    ln -sf /etc/runit/edgelet /var/service/edgelet 2>/dev/null || true
                sv restart edgelet 2>/dev/null || sv start edgelet 2>/dev/null || true
            fi
            info "runit service installed under /etc/runit/edgelet."
            ;;
        *)
            die "No supported init system detected (${_init}). Install systemd, procd, openrc, sysvinit, upstart, s6, or runit."
            ;;
    esac
}

stop_edgelet_service() {
    _init="${1:-$(detect_init)}"
    case "${_init}" in
        systemd) systemctl stop edgelet 2>/dev/null || true ;;
        openrc) rc-service edgelet stop 2>/dev/null || true ;;
        procd) /etc/init.d/edgelet stop 2>/dev/null || true ;;
        sysvinit) /etc/init.d/edgelet stop 2>/dev/null || true ;;
        upstart) initctl stop edgelet 2>/dev/null || true ;;
        s6) s6-svc -d /var/run/s6/services/edgelet 2>/dev/null || true ;;
        runit) sv down edgelet 2>/dev/null || true ;;
        *) "${EDGELET_LIBEXEC}/edgelet-shutdown" 2>/dev/null || pkill -f "/usr/local/bin/edgelet daemon" 2>/dev/null || true ;;
    esac
}
