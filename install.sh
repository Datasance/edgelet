#!/bin/sh
# install.sh — ioFog Agent universal installer (tarball-only)
#
# Usage:
#   curl -fsSL ... | sudo sh -s -- --flavor=full
#   sudo sh install.sh --flavor=full [--version=vX.Y.Z]
#   sudo sh install.sh --airgap --tarball-path=/path/to/iofog-agent-...-full.tar.gz
#   sudo sh install.sh --upgrade [--version=vX.Y.Z]
#   sudo sh install.sh --rollback [--airgap --tarball-path=...]
#
# Default: --flavor=full (embedded containerd / iofog engine build)

set -e

die() { echo "ERROR: $1" >&2; exit 1; }
info() { echo ">>> $1"; }

require_root() {
    [ "$(id -u)" -eq 0 ] || die "This script must be run as root. Try: sudo $0 $*"
}

BACKUP_DIR="/var/backups/iofog-agent"
# POSIX key=value files (no JSON, no python3)
RECEIPT_FILE="${BACKUP_DIR}/install-receipt"
PREVIOUS_FILE="${BACKUP_DIR}/previous-release"
GITHUB_REPO="datasance/agent"

detect_arch() {
    MACHINE=$(uname -m)
    case "${MACHINE}" in
        x86_64)   echo "amd64" ;;
        aarch64)  echo "arm64" ;;
        armv7l|armv6l|arm) echo "arm" ;;
        riscv64)  echo "riscv64" ;;
        *) die "Unsupported architecture: ${MACHINE}" ;;
    esac
}

detect_libc() {
    if [ -f /lib/ld-musl-*.so.1 ] || [ -f /usr/lib/ld-musl-*.so.1 ]; then
        echo "musl"
    else
        echo "glibc"
    fi
}

detect_init() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /etc/systemd/system ]; then
        echo "systemd"
    elif command -v openrc >/dev/null 2>&1 || [ -f /sbin/openrc ]; then
        echo "openrc"
    elif [ -f /sbin/init ] && /sbin/init --version 2>/dev/null | grep -q upstart; then
        echo "upstart"
    elif [ -d /etc/s6 ] || command -v s6-svc >/dev/null 2>&1; then
        echo "s6"
    elif command -v runit >/dev/null 2>&1 || [ -d /etc/runit ]; then
        echo "runit"
    elif [ -d /etc/init.d ]; then
        echo "sysvinit"
    else
        echo "unknown"
    fi
}

# ── optional checksum (airgap) ────────────────────────────────────────────────
EXPECTED_SHA256=""
CHECKSUM_FILE=""

verify_tarball_checksum() {
    _tar="$1"
    [ -f "$_tar" ] || die "Not a file: $_tar"
    if [ -n "$EXPECTED_SHA256" ]; then
        _sum=$(sha256sum "$_tar" | awk '{print $1}')
        [ "$_sum" = "$EXPECTED_SHA256" ] || die "SHA256 mismatch (expected $EXPECTED_SHA256 got $_sum)"
        info "SHA256 verified."
    elif [ -n "$CHECKSUM_FILE" ] && [ -f "$CHECKSUM_FILE" ]; then
        ( cd "$(dirname "$_tar")" && sha256sum -c "$CHECKSUM_FILE" ) || die "Checksum file verification failed"
    fi
}

# ── Key=value metadata (POSIX sh; one line per key; value may contain '=') ─────
kv_get() {
    _file="$1" _key="$2"
    [ -f "$_file" ] || { echo ""; return 0; }
    _line=$(grep "^${_key}=" "$_file" | head -1) || true
    [ -n "$_line" ] || { echo ""; return 0; }
    echo "$_line" | sed "s/^${_key}=//"
}

write_install_receipt() {
    _ver="$1" _flavor="$2" _url="$3"
    mkdir -p "$BACKUP_DIR"
    {
        printf 'installed_version=%s\n' "$_ver"
        printf 'flavor=%s\n' "$_flavor"
        printf 'source_url=%s\n' "$_url"
    } >"$RECEIPT_FILE"
    chmod 600 "$RECEIPT_FILE" 2>/dev/null || true
}

write_previous_release() {
    _pv="$1" _pf="$2" _purl="$3" _cfg="$4"
    mkdir -p "$BACKUP_DIR"
    {
        printf 'previous_version=%s\n' "$_pv"
        printf 'previous_flavor=%s\n' "$_pf"
        printf 'previous_download_url=%s\n' "$_purl"
        printf 'config_backup_path=%s\n' "$_cfg"
    } >"$PREVIOUS_FILE"
    chmod 600 "$PREVIOUS_FILE" 2>/dev/null || true
}

# ── argument parsing ──────────────────────────────────────────────────────────
IOFOG_VERSION="${IOFOG_VERSION:-latest}"
IOFOG_FLAVOR="${IOFOG_FLAVOR:-full}"
CONTAINER_ENGINE=""
ACTION="install"
AIRGAP=false
TARBALL_PATH=""
FORCE_CONFIG=false
BIN_PATH_LEGACY=""

for arg in "$@"; do
    case "${arg}" in
        --version=*)          IOFOG_VERSION="${arg#*=}" ;;
        --flavor=*)           IOFOG_FLAVOR="${arg#*=}" ;;
        --container-engine=*) CONTAINER_ENGINE="${arg#*=}" ;;
        --bin-path=*)         BIN_PATH_LEGACY="${arg#*=}" ;;
        --tarball-path=*)     TARBALL_PATH="${arg#*=}" ;;
        --checksum-path=*)    CHECKSUM_FILE="${arg#*=}" ;;
        --expected-sha256=*)  EXPECTED_SHA256="${arg#*=}" ;;
        --airgap)             AIRGAP=true ;;
        --upgrade)            ACTION="upgrade" ;;
        --rollback)           ACTION="rollback" ;;
        --force-config)       FORCE_CONFIG=true ;;
        --help|-h)
            cat <<EOF
Usage: $0 [options]

Options:
  --flavor=full|lite         Build flavor (default: full). full = embedded containerd; lite = docker/podman only.
  --version=VERSION          Release tag (default: latest)
  --container-engine=ENGINE  For lite: docker or podman (default: docker). Ignored for full (always iofog).
  --airgap                   Do not download; use --tarball-path
  --tarball-path=PATH        Local tarball (.tar.gz)
  --bin-path=PATH            Alias for --tarball-path (legacy)
  --checksum-path=PATH       Optional sha256sum file for verification
  --expected-sha256=HASH     Optional expected SHA256 of tarball
  --upgrade                  Upgrade existing install (writes previous-release)
  --rollback                 Roll back using previous-release metadata file
  --force-config             Replace config on upgrade/rollback (default: preserve / restore)

Environment:
  IOFOG_VERSION   IOFOG_FLAVOR
EOF
            exit 0 ;;
        *) die "Unknown option: ${arg} (use --help)" ;;
    esac
done

[ -n "$BIN_PATH_LEGACY" ] && TARBALL_PATH="${TARBALL_PATH:-$BIN_PATH_LEGACY}"

if [ "$AIRGAP" = true ] && [ -z "$TARBALL_PATH" ]; then
    die "--airgap requires --tarball-path"
fi

if [ "$ACTION" != "rollback" ]; then
    case "$IOFOG_FLAVOR" in
        full|lite) ;;
        *) die "Invalid --flavor (use full or lite)" ;;
    esac

    # Default engine for lite
    if [ -z "$CONTAINER_ENGINE" ]; then
        if [ "$IOFOG_FLAVOR" = "full" ]; then
            CONTAINER_ENGINE="iofog"
        else
            CONTAINER_ENGINE="docker"
        fi
    fi

    if [ "$IOFOG_FLAVOR" = "full" ] && [ "$CONTAINER_ENGINE" != "iofog" ]; then
        die "full flavor requires containerEngine iofog (embedded containerd)"
    fi
    if [ "$IOFOG_FLAVOR" = "lite" ] && [ "$CONTAINER_ENGINE" = "iofog" ]; then
        die "lite flavor cannot use iofog engine; use full flavor"
    fi
fi

require_root

ARCH=$(detect_arch)
LIBC=$(detect_libc)
INIT=$(detect_init)

info "Architecture : ${ARCH}"
info "Libc         : ${LIBC}"
info "Init system  : ${INIT}"
if [ "$ACTION" != "rollback" ]; then
    info "Flavor       : ${IOFOG_FLAVOR}"
fi
info "Action       : ${ACTION}"

ARCH_SUFFIX="${ARCH}"
if [ "${LIBC}" = "musl" ] && { [ "${ARCH}" = "amd64" ] || [ "${ARCH}" = "arm64" ]; }; then
    ARCH_SUFFIX="${ARCH}-musl"
fi

TARBALL_BASENAME="iofog-agent-${IOFOG_VERSION}-linux-${ARCH_SUFFIX}-${IOFOG_FLAVOR}.tar.gz"
TMPDIR=$(mktemp -d)
trap 'rm -rf "${TMPDIR}"' EXIT

# ── resolve version for remote download ───────────────────────────────────────
if [ "$AIRGAP" = false ] && [ "$IOFOG_VERSION" = "latest" ] && [ "$ACTION" != "rollback" ]; then
    info "Fetching latest release tag..."
    IOFOG_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    [ -n "${IOFOG_VERSION}" ] || die "Failed to determine latest version"
    TARBALL_BASENAME="iofog-agent-${IOFOG_VERSION}-linux-${ARCH_SUFFIX}-${IOFOG_FLAVOR}.tar.gz"
fi

info "Version: ${IOFOG_VERSION}"

# ── download or use local tarball ─────────────────────────────────────────────
download_or_copy_tarball() {
    _dest="$1"
    if [ "$AIRGAP" = true ]; then
        [ -f "$TARBALL_PATH" ] || die "Local archive not found: $TARBALL_PATH"
        verify_tarball_checksum "$TARBALL_PATH"
        cp "$TARBALL_PATH" "$_dest"
        info "Using local archive: ${TARBALL_PATH}"
    else
        _url="https://github.com/${GITHUB_REPO}/releases/download/${IOFOG_VERSION}/${TARBALL_BASENAME}"
        info "Downloading ${_url} ..."
        curl -fsSL -o "$_dest" "$_url" || die "Failed to download ${_url}"
    fi
}

extract_tarball() {
    _tg="$1"
    info "Extracting archive..."
    tar -xzf "$_tg" -C "${TMPDIR}"
}

install_binaries() {
    install -m 755 "${TMPDIR}/iofog-agent"  /usr/local/bin/iofog-agent
    install -m 755 "${TMPDIR}/iofog-agentd" /usr/local/bin/iofog-agentd
    info "Binaries installed to /usr/local/bin/"
}

install_cli_completion() {
    if ! command -v iofog-agent >/dev/null 2>&1; then
        return 0
    fi
    if [ -d /etc/bash_completion.d ]; then
        if iofog-agent completion bash >/etc/bash_completion.d/iofog-agent 2>/dev/null; then
            chmod 644 /etc/bash_completion.d/iofog-agent
            info "Bash completion installed to /etc/bash_completion.d/iofog-agent"
            return 0
        fi
    fi
    if [ -f packaging/iofog-agent/etc/bash_completion.d/iofog-agent ]; then
        install -m 644 packaging/iofog-agent/etc/bash_completion.d/iofog-agent /etc/bash_completion.d/iofog-agent 2>/dev/null || true
    fi
}

default_docker_url_for_engine() {
    case "$1" in
        docker) echo "unix:///var/run/docker.sock" ;;
        podman) echo "unix:///run/podman/podman.sock" ;;
        iofog)  echo "unix:///run/iofog-agent/containerd.sock" ;;
        *) die "Unsupported engine: $1" ;;
    esac
}

write_default_config_if_missing() {
    if [ ! -f /etc/iofog-agent/config.yaml ]; then
        _du=$(default_docker_url_for_engine "$CONTAINER_ENGINE")
        cat >/etc/iofog-agent/config.yaml <<YAML
currentProfile: production
profiles:
  production:
    containerEngine: ${CONTAINER_ENGINE}
    dockerUrl: ${_du}
    diskDirectory: /var/lib/iofog-agent/
    logDirectory: /var/log/iofog-agent/
    logLevel: INFO
YAML
        info "Default config installed at /etc/iofog-agent/config.yaml"
    else
        info "Existing config preserved at /etc/iofog-agent/config.yaml"
    fi
}

install_dirs() {
    mkdir -p /etc/iofog-agent
    chmod 750 /etc/iofog-agent
    mkdir -p /var/log/iofog-agent /var/lib/iofog-agent /var/run/iofog-agent \
        "$BACKUP_DIR" /var/lib/iofog-agent-containerd
    chmod 750 /var/log/iofog-agent /var/lib/iofog-agent /var/run/iofog-agent \
        "$BACKUP_DIR" /var/lib/iofog-agent-containerd 2>/dev/null || true
}

# ── init: stop/start helpers ──────────────────────────────────────────────────
stop_agent_service() {
    case "${INIT}" in
        systemd)
            systemctl stop iofog-agentd 2>/dev/null || true
            ;;
        openrc)
            rc-service iofog-agentd stop 2>/dev/null || true
            ;;
        sysvinit)
            /etc/init.d/iofog-agentd stop 2>/dev/null || true
            ;;
        upstart)
            initctl stop iofog-agentd 2>/dev/null || true
            ;;
        *)
            pkill -f "iofog-agentd" 2>/dev/null || true
            ;;
    esac
}

start_agent_service() {
    case "${INIT}" in
        systemd)
            systemctl daemon-reload 2>/dev/null || true
            systemctl enable iofog-agentd 2>/dev/null || true
            systemctl start iofog-agentd 2>/dev/null || true
            ;;
        openrc)
            rc-service iofog-agentd start 2>/dev/null || true
            ;;
        sysvinit)
            /etc/init.d/iofog-agentd start 2>/dev/null || true
            ;;
        upstart)
            initctl start iofog-agentd 2>/dev/null || true
            ;;
        *)
            nohup /usr/local/bin/iofog-agentd daemon >/var/log/iofog-agentd.log 2>&1 &
            ;;
    esac
}

# ── systemd unit ─────────────────────────────────────────────────────────────
install_systemd() {
    _full="$1"
    _eng="$2"
    _after="network-online.target"
    _wants=""
    case "${_eng}" in
        docker)
            _after="network-online.target docker.service"
            _wants="Wants=docker.service"
            ;;
        podman)
            _after="network-online.target podman.socket"
            _wants="Wants=podman.socket"
            ;;
        iofog)
            _after="network-online.target"
            ;;
    esac

    if [ "$_full" = "true" ]; then
        cat >/etc/systemd/system/iofog-agentd.service <<EOF
[Unit]
Description=ioFog Agent Daemon (full / embedded containerd)
${_wants}
After=${_after}
StartLimitIntervalSec=300
StartLimitBurst=20

[Service]
Type=simple
ExecStart=/usr/local/bin/iofog-agentd start
Restart=always
RestartSec=2s
StandardOutput=journal
StandardError=journal
LimitNOFILE=65536
TimeoutStopSec=120
KillMode=control-group
KillSignal=SIGTERM
SendSIGKILL=yes
NoNewPrivileges=no
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/etc/iofog-agent /var/lib/iofog-agent /var/lib/iofog-agent-containerd /run/iofog-agent /var/log/iofog-agent /var/backups/iofog-agent
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_SYS_CHROOT CAP_DAC_OVERRIDE CAP_SETGID CAP_SETUID CAP_DAC_READ_SEARCH
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
LockPersonality=true
MemoryDenyWriteExecute=true

[Install]
WantedBy=multi-user.target
EOF
    else
        cat >/etc/systemd/system/iofog-agentd.service <<EOF
[Unit]
Description=ioFog Agent Daemon (lite)
${_wants}
After=${_after}
StartLimitIntervalSec=300
StartLimitBurst=20

[Service]
Type=simple
ExecStart=/usr/local/bin/iofog-agentd start
Restart=always
RestartSec=2s
StandardOutput=journal
StandardError=journal
LimitNOFILE=65536
TimeoutStopSec=120
KillMode=control-group
KillSignal=SIGTERM
SendSIGKILL=yes
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/etc/iofog-agent /var/lib/iofog-agent /var/run/iofog-agent /var/log/iofog-agent /var/backups/iofog-agent
CapabilityBoundingSet=CAP_NET_BIND_SERVICE CAP_SYS_CHROOT CAP_DAC_OVERRIDE CAP_SETGID CAP_SETUID CAP_DAC_READ_SEARCH
RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6 AF_NETLINK
LockPersonality=true

[Install]
WantedBy=multi-user.target
EOF
    fi
    systemctl daemon-reload
    systemctl enable iofog-agentd
    systemctl restart iofog-agentd
    info "systemd service installed and started."
}

install_openrc() {
    cat >/etc/init.d/iofog-agentd <<'EOF'
#!/sbin/openrc-run
description="ioFog Agent Daemon"
command="/usr/local/bin/iofog-agentd"
command_args="daemon"
command_background=true
pidfile="/run/iofog-agentd.pid"
EOF
    chmod +x /etc/init.d/iofog-agentd
    rc-update add iofog-agentd default
    rc-service iofog-agentd start
    info "OpenRC service installed and started."
}

install_sysvinit() {
    cat >/etc/init.d/iofog-agentd <<'EOF'
#!/bin/sh
### BEGIN INIT INFO
# Provides:          iofog-agentd
# Required-Start:    $network
# Required-Stop:     $network
# Default-Start:     2 3 4 5
# Default-Stop:      0 1 6
# Short-Description: ioFog Agent Daemon
### END INIT INFO

DAEMON=/usr/local/bin/iofog-agentd
PIDFILE=/var/run/iofog-agentd.pid

case "$1" in
  start)
    start-stop-daemon --start --background --make-pidfile --pidfile $PIDFILE \
      --exec $DAEMON -- daemon ;;
  stop)
    start-stop-daemon --stop --pidfile $PIDFILE ;;
  restart)
    $0 stop; $0 start ;;
  status)
    if [ -f $PIDFILE ] && kill -0 "$(cat $PIDFILE)" 2>/dev/null; then
      echo "iofog-agentd is running"
    else
      echo "iofog-agentd is not running"
    fi ;;
  *)
    echo "Usage: $0 {start|stop|restart|status}" ;;
esac
EOF
    chmod +x /etc/init.d/iofog-agentd
    update-rc.d iofog-agentd defaults 2>/dev/null || true
    /etc/init.d/iofog-agentd start
    info "SysV init service installed and started."
}

install_upstart() {
    cat >/etc/init/iofog-agentd.conf <<'EOF'
description "ioFog Agent Daemon"
start on runlevel [2345]
stop on runlevel [016]
respawn
exec /usr/local/bin/iofog-agentd daemon
EOF
    initctl reload-configuration 2>/dev/null || true
    initctl start iofog-agentd
    info "Upstart service installed and started."
}

install_s6() {
    SVCDIR=/etc/s6/iofog-agentd
    mkdir -p "${SVCDIR}"
    cat >"${SVCDIR}/run" <<'EOF'
#!/bin/execlineb -P
/usr/local/bin/iofog-agentd daemon
EOF
    chmod +x "${SVCDIR}/run"
    ln -sf "${SVCDIR}" /var/run/s6/services/iofog-agentd 2>/dev/null || true
    s6-svscanctl -a /var/run/s6/services 2>/dev/null || true
    info "s6 service installed."
}

install_runit() {
    SVCDIR=/etc/runit/iofog-agentd
    mkdir -p "${SVCDIR}"
    cat >"${SVCDIR}/run" <<'EOF'
#!/bin/sh
exec /usr/local/bin/iofog-agentd daemon
EOF
    chmod +x "${SVCDIR}/run"
    ln -sf "${SVCDIR}" /var/service/iofog-agentd 2>/dev/null || \
        ln -sf "${SVCDIR}" /service/iofog-agentd 2>/dev/null || true
    info "runit service installed."
}

install_service_for_init() {
    _is_full="$1"
    case "${INIT}" in
        systemd)   install_systemd "$_is_full" "$CONTAINER_ENGINE" ;;
        openrc)    install_openrc ;;
        sysvinit)  install_sysvinit ;;
        upstart)   install_upstart ;;
        s6)        install_s6 ;;
        runit)     install_runit ;;
        *)
            info "Unknown init system. Starting iofog-agentd in background."
            nohup /usr/local/bin/iofog-agentd daemon >/var/log/iofog-agentd.log 2>&1 &
            ;;
    esac
}

# ── source URL for receipt ────────────────────────────────────────────────────
compute_source_url() {
    if [ "$AIRGAP" = true ]; then
        # file:/// absolute path
        _real="$(cd "$(dirname "$TARBALL_PATH")" && pwd)/$(basename "$TARBALL_PATH")"
        echo "file://${_real}"
    else
        echo "https://github.com/${GITHUB_REPO}/releases/download/${IOFOG_VERSION}/${TARBALL_BASENAME}"
    fi
}

# ═══════════════════════════════════════════════════════════════════════════════
# rollback
# ═══════════════════════════════════════════════════════════════════════════════
if [ "$ACTION" = "rollback" ]; then
    [ -f "$PREVIOUS_FILE" ] || die "No ${PREVIOUS_FILE} found. Cannot rollback."
    _pv=$(kv_get "$PREVIOUS_FILE" "previous_version")
    _pf=$(kv_get "$PREVIOUS_FILE" "previous_flavor")
    _purl=$(kv_get "$PREVIOUS_FILE" "previous_download_url")
    _cfgbak=$(kv_get "$PREVIOUS_FILE" "config_backup_path")
    [ -n "$_pv" ] || die "previous_version missing or empty in ${PREVIOUS_FILE}"
    [ -n "$_pf" ] || die "previous_flavor missing or empty in ${PREVIOUS_FILE}"
    IOFOG_FLAVOR="$_pf"
    IOFOG_VERSION="$_pv"
    TARBALL_BASENAME="iofog-agent-${IOFOG_VERSION}-linux-${ARCH_SUFFIX}-${IOFOG_FLAVOR}.tar.gz"
    CONTAINER_ENGINE="iofog"
    [ "$IOFOG_FLAVOR" = "lite" ] && CONTAINER_ENGINE="docker"

    stop_agent_service

    _tg="${TMPDIR}/rollback.tar.gz"
    if [ "$AIRGAP" = true ]; then
        [ -n "$TARBALL_PATH" ] || die "rollback with --airgap requires --tarball-path"
        verify_tarball_checksum "$TARBALL_PATH"
        cp "$TARBALL_PATH" "$_tg"
    else
        case "$_purl" in
            http://*|https://*)
                info "Downloading rollback from ${_purl}"
                curl -fsSL -o "$_tg" "$_purl" || die "Failed to download ${_purl}"
                ;;
            file://*)
                _fp=$(echo "$_purl" | sed 's|^file://||')
                [ -f "$_fp" ] || die "Rollback file not found: $_fp"
                cp "$_fp" "$_tg"
                ;;
            *)
                die "Cannot rollback online: unsupported previous_download_url (use --airgap --tarball-path)"
                ;;
        esac
    fi

    extract_tarball "$_tg"
    install_binaries
    install_dirs

    if [ "$FORCE_CONFIG" != true ] && [ -f "$_cfgbak" ]; then
        install -m 640 "$_cfgbak" /etc/iofog-agent/config.yaml
        info "Restored config from ${_cfgbak}"
    fi

    _is_full="false"
    [ "$IOFOG_FLAVOR" = "full" ] && _is_full="true"
    install_service_for_init "$_is_full"

    _src="$_purl"
    write_install_receipt "$IOFOG_VERSION" "$IOFOG_FLAVOR" "$_src"

    info "Rollback to ${IOFOG_VERSION} (${IOFOG_FLAVOR}) complete."
    exit 0
fi

# ═══════════════════════════════════════════════════════════════════════════════
# upgrade
# ═══════════════════════════════════════════════════════════════════════════════
if [ "$ACTION" = "upgrade" ]; then
    [ -f /usr/local/bin/iofog-agentd ] || die "Agent not installed; run install first"
    [ -f "$RECEIPT_FILE" ] || die "Missing ${RECEIPT_FILE}; cannot determine current version"

    _cur_ver=$(kv_get "$RECEIPT_FILE" "installed_version")
    _cur_fl=$(kv_get "$RECEIPT_FILE" "flavor")
    _cur_src=$(kv_get "$RECEIPT_FILE" "source_url")

    [ -n "$_cur_ver" ] || die "installed_version missing or empty in ${RECEIPT_FILE}"
    [ -n "$_cur_fl" ] || die "flavor missing or empty in ${RECEIPT_FILE}"

    [ "$_cur_fl" = "$IOFOG_FLAVOR" ] || die "Flavor mismatch: installed ${_cur_fl}, requested ${IOFOG_FLAVOR}"

    if [ "$IOFOG_VERSION" = "latest" ] || [ -z "$IOFOG_VERSION" ]; then
        IOFOG_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    fi
    TARBALL_BASENAME="iofog-agent-${IOFOG_VERSION}-linux-${ARCH_SUFFIX}-${IOFOG_FLAVOR}.tar.gz"

    _cfg_backup="${BACKUP_DIR}/config.yaml.$(date +%Y%m%d%H%M%S)"
    cp /etc/iofog-agent/config.yaml "$_cfg_backup" 2>/dev/null || true

    write_previous_release "$_cur_ver" "$_cur_fl" "$_cur_src" "$_cfg_backup"

    stop_agent_service

    _tg="${TMPDIR}/upgrade.tar.gz"
    download_or_copy_tarball "$_tg"
    extract_tarball "$_tg"
    install_binaries
    install_dirs

    if [ "$FORCE_CONFIG" = true ]; then
        rm -f /etc/iofog-agent/config.yaml
        write_default_config_if_missing
    fi

    _src=$(compute_source_url)
    write_install_receipt "$IOFOG_VERSION" "$IOFOG_FLAVOR" "$_src"

    _is_full="false"
    [ "$IOFOG_FLAVOR" = "full" ] && _is_full="true"
    install_service_for_init "$_is_full"

    info "Upgrade to ${IOFOG_VERSION} complete."
    exit 0
fi

# ═══════════════════════════════════════════════════════════════════════════════
# fresh install
# ═══════════════════════════════════════════════════════════════════════════════
_tg="${TMPDIR}/${TARBALL_BASENAME}"
download_or_copy_tarball "$_tg"
extract_tarball "$_tg"
install_dirs
install_binaries

if [ -f "${TMPDIR}/config.yaml.sample" ] && [ ! -f /etc/iofog-agent/config.yaml ]; then
    install -m 640 "${TMPDIR}/config.yaml.sample" /etc/iofog-agent/config.yaml
    info "Sample config installed from tarball"
else
    write_default_config_if_missing
fi

if command -v sed >/dev/null 2>&1 && grep -q "containerEngine" /etc/iofog-agent/config.yaml 2>/dev/null; then
    sed -i "s|containerEngine:.*|containerEngine: ${CONTAINER_ENGINE}|" /etc/iofog-agent/config.yaml
fi
_durl=$(default_docker_url_for_engine "$CONTAINER_ENGINE")
if command -v sed >/dev/null 2>&1 && grep -q "dockerUrl" /etc/iofog-agent/config.yaml 2>/dev/null; then
    sed -i "s|dockerUrl:.*|dockerUrl: ${_durl}|" /etc/iofog-agent/config.yaml
fi

_src=$(compute_source_url)
write_install_receipt "$IOFOG_VERSION" "$IOFOG_FLAVOR" "$_src"

_is_full="false"
[ "$IOFOG_FLAVOR" = "full" ] && _is_full="true"
install_service_for_init "$_is_full"
install_cli_completion

info ""
info "iofog-agent ${IOFOG_VERSION} (${IOFOG_FLAVOR}) installed successfully."
info "  CLI    : /usr/local/bin/iofog-agent"
info "  Daemon : /usr/local/bin/iofog-agentd"
info "  Config : /etc/iofog-agent/config.yaml"
info ""
info "To check status: iofog-agent system status"
info "To provision:    iofog-agent provision <provisioning-key>"
info "Shell completion: source /etc/bash_completion.d/iofog-agent (bash)"
info ""
