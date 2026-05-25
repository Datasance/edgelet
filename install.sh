#!/bin/sh
# install.sh — Edgelet greenfield installer (tarball-only)
#
# Usage:
#   curl -fsSL ... | sudo sh -s -- --flavor=full
#   sudo ./install.sh --flavor=full [--version=vX.Y.Z]
#   sudo ./install.sh --airgap --tarball-path=dist/edgelet-linux-amd64-full.tar.gz
#   sudo ./install.sh --flavor=lite --container-engine=docker
#
# Linux gates (CI / VM): systemctl status edgelet
# macOS dev for embed CI: make ci-docker  (runs scripts/ci in Docker)

set -e

die() { echo "ERROR: $1" >&2; exit 1; }
info() { echo ">>> $1"; }

require_root() {
    [ "$(id -u)" -eq 0 ] || die "This script must be run as root. Try: sudo $0 $*"
}

BACKUP_DIR="/var/backups/edgelet"
RECEIPT_FILE="${BACKUP_DIR}/install-receipt"
PREVIOUS_FILE="${BACKUP_DIR}/previous-release"
GITHUB_REPO="${EDGELET_GITHUB_REPO:-datasance/edgelet}"
UNIT_NAME="edgelet"
BINARY_PATH="/usr/local/bin/edgelet"
CONFIG_DIR="/etc/edgelet"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"

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

detect_init() {
    if command -v systemctl >/dev/null 2>&1 && [ -d /etc/systemd/system ]; then
        echo "systemd"
    elif command -v openrc >/dev/null 2>&1 || [ -f /sbin/openrc ]; then
        echo "openrc"
    else
        echo "unknown"
    fi
}

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
EDGELET_VERSION="${EDGELET_VERSION:-latest}"
EDGELET_FLAVOR="${EDGELET_FLAVOR:-full}"
CONTAINER_ENGINE=""
ACTION="install"
AIRGAP=false
TARBALL_PATH=""
FORCE_CONFIG=false
NON_INTERACTIVE=false
CONTROLLER_URL=""
PROVISION_KEY=""
ARCH_OVERRIDE=""
BIN_PATH_LEGACY=""

for arg in "$@"; do
    case "${arg}" in
        --version=*)          EDGELET_VERSION="${arg#*=}" ;;
        --flavor=*)           EDGELET_FLAVOR="${arg#*=}" ;;
        --arch=*)             ARCH_OVERRIDE="${arg#*=}" ;;
        --container-engine=*) CONTAINER_ENGINE="${arg#*=}" ;;
        --bin-path=*)         BIN_PATH_LEGACY="${arg#*=}" ;;
        --tarball-path=*)     TARBALL_PATH="${arg#*=}" ;;
        --checksum-path=*)    CHECKSUM_FILE="${arg#*=}" ;;
        --expected-sha256=*)  EXPECTED_SHA256="${arg#*=}" ;;
        --airgap)             AIRGAP=true ;;
        --upgrade)            ACTION="upgrade" ;;
        --rollback)           ACTION="rollback" ;;
        --force-config)       FORCE_CONFIG=true ;;
        --non-interactive)    NON_INTERACTIVE=true ;;
        --controller-url=*)   CONTROLLER_URL="${arg#*=}" ;;
        --provision-key=*)    PROVISION_KEY="${arg#*=}" ;;
        --help|-h)
            cat <<EOF
Usage: $0 [options]

Options:
  --flavor=full|lite         Build flavor (default: full)
  --version=VERSION          Release tag (default: latest)
  --arch=ARCH                Override auto-detected arch (amd64, arm64, arm, riscv64)
  --container-engine=ENGINE  Lite only: docker or podman (default: docker)
  --airgap                   Do not download; use --tarball-path
  --tarball-path=PATH        Local edgelet-*.tar.gz
  --checksum-path=PATH       Optional sha256sum manifest
  --expected-sha256=HASH     Optional tarball SHA256
  --upgrade / --rollback     In-place upgrade or rollback (same flavor)
  --force-config             Replace config on upgrade/rollback
  --non-interactive          Pot-oriented: no prompts
  --controller-url=URL       Optional: write controllerUrl into new config
  --provision-key=KEY        Optional: run edgelet provision after install

Environment:
  EDGELET_VERSION  EDGELET_FLAVOR  EDGELET_GITHUB_REPO
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
    case "$EDGELET_FLAVOR" in
        full|lite) ;;
        *) die "Invalid --flavor (use full or lite)" ;;
    esac

    if [ -z "$CONTAINER_ENGINE" ]; then
        if [ "$EDGELET_FLAVOR" = "full" ]; then
            CONTAINER_ENGINE="edgelet"
        else
            CONTAINER_ENGINE="docker"
        fi
    fi

    if [ "$EDGELET_FLAVOR" = "full" ] && [ "$CONTAINER_ENGINE" != "edgelet" ]; then
        die "full flavor requires containerEngine edgelet (embedded containerd)"
    fi
    if [ "$EDGELET_FLAVOR" = "lite" ] && [ "$CONTAINER_ENGINE" = "edgelet" ]; then
        die "lite flavor cannot use edgelet engine; use --flavor=full"
    fi
fi

require_root

ARCH="${ARCH_OVERRIDE:-$(detect_arch)}"
INIT=$(detect_init)

info "Architecture : ${ARCH}"
info "Init system  : ${INIT}"
if [ "$ACTION" != "rollback" ]; then
    info "Flavor       : ${EDGELET_FLAVOR}"
fi
info "Action       : ${ACTION}"

tarball_name_for() {
    _ver="$1"
    _arch="$2"
    _fl="$3"
    echo "edgelet-${_ver}-linux-${_arch}-${_fl}.tar.gz"
}

resolve_tarball_basename() {
    _ver="$1"
    _arch="$2"
    _fl="$3"
    if [ "$AIRGAP" = true ] && [ -n "$TARBALL_PATH" ]; then
        basename "$TARBALL_PATH"
        return 0
    fi
    tarball_name_for "$_ver" "$_arch" "$_fl"
}

TARBALL_BASENAME=$(tarball_name_for "${EDGELET_VERSION}" "${ARCH}" "${EDGELET_FLAVOR}")
TMPDIR=$(mktemp -d)
trap 'rm -rf "${TMPDIR}"' EXIT

if [ "$AIRGAP" = false ] && [ "$EDGELET_VERSION" = "latest" ] && [ "$ACTION" != "rollback" ]; then
    info "Fetching latest release tag..."
    EDGELET_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    [ -n "${EDGELET_VERSION}" ] || die "Failed to determine latest version"
    TARBALL_BASENAME=$(tarball_name_for "${EDGELET_VERSION}" "${ARCH}" "${EDGELET_FLAVOR}")
fi

info "Version: ${EDGELET_VERSION}"

download_or_copy_tarball() {
    _dest="$1"
    if [ "$AIRGAP" = true ]; then
        [ -f "$TARBALL_PATH" ] || die "Local archive not found: $TARBALL_PATH"
        verify_tarball_checksum "$TARBALL_PATH"
        cp "$TARBALL_PATH" "$_dest"
        info "Using local archive: ${TARBALL_PATH}"
        return 0
    fi
    _url="https://github.com/${GITHUB_REPO}/releases/download/${EDGELET_VERSION}/${TARBALL_BASENAME}"
    info "Downloading ${_url} ..."
    if ! curl -fsSL -o "$_dest" "$_url"; then
        _alt="edgelet-linux-${ARCH}-${EDGELET_FLAVOR}.tar.gz"
        _url="https://github.com/${GITHUB_REPO}/releases/download/${EDGELET_VERSION}/${_alt}"
        info "Retrying ${_url} ..."
        curl -fsSL -o "$_dest" "$_url" || die "Failed to download release tarball"
    fi
}

extract_tarball() {
    _tg="$1"
    info "Extracting archive..."
    tar -xzf "$_tg" -C "${TMPDIR}"
}

install_binary() {
    if [ ! -f "${TMPDIR}/edgelet" ]; then
        die "Tarball missing edgelet binary"
    fi
    install -m 755 "${TMPDIR}/edgelet" "${BINARY_PATH}"
    info "Installed ${BINARY_PATH}"
}

install_cli_completion() {
    if ! command -v edgelet >/dev/null 2>&1; then
        return 0
    fi
    if [ -d /etc/bash_completion.d ]; then
        if edgelet completion bash >/etc/bash_completion.d/edgelet 2>/dev/null; then
            chmod 644 /etc/bash_completion.d/edgelet
            info "Bash completion installed to /etc/bash_completion.d/edgelet"
            return 0
        fi
    fi
    if [ -f packaging/edgelet/etc/bash_completion.d/edgelet ]; then
        install -m 644 packaging/edgelet/etc/bash_completion.d/edgelet /etc/bash_completion.d/edgelet 2>/dev/null || true
    fi
}

default_docker_url_for_engine() {
    case "$1" in
        docker) echo "unix:///var/run/docker.sock" ;;
        podman) echo "unix:///run/podman/podman.sock" ;;
        edgelet) echo "unix:///run/edgelet/containerd.sock" ;;
        *) die "Unsupported engine: $1" ;;
    esac
}

write_default_config_if_missing() {
    if [ ! -f "$CONFIG_FILE" ]; then
        _du=$(default_docker_url_for_engine "$CONTAINER_ENGINE")
        _ctrl="${CONTROLLER_URL:-http://localhost:54421/api/v3/}"
        cat >"$CONFIG_FILE" <<YAML
currentProfile: production
profiles:
  production:
    controllerUrl: "${_ctrl}"
    containerEngine: ${CONTAINER_ENGINE}
    dockerUrl: ${_du}
    diskDirectory: /var/lib/edgelet/
    logDiskDirectory: /var/log/edgelet/
    logLevel: INFO
YAML
        chmod 640 "$CONFIG_FILE"
        info "Default config installed at ${CONFIG_FILE}"
    else
        info "Existing config preserved at ${CONFIG_FILE}"
    fi
}

install_dirs() {
    mkdir -p "$CONFIG_DIR" /var/log/edgelet /var/lib/edgelet /run/edgelet \
        /var/lib/edgelet-containerd "$BACKUP_DIR"
    chmod 750 "$CONFIG_DIR" /var/log/edgelet /var/lib/edgelet 2>/dev/null || true
}

stop_edgelet_service() {
    case "${INIT}" in
        systemd) systemctl stop "${UNIT_NAME}" 2>/dev/null || true ;;
        openrc) rc-service "${UNIT_NAME}" stop 2>/dev/null || true ;;
        *) pkill -f "${BINARY_PATH}" 2>/dev/null || true ;;
    esac
}

start_edgelet_service() {
    case "${INIT}" in
        systemd)
            systemctl daemon-reload 2>/dev/null || true
            systemctl enable "${UNIT_NAME}" 2>/dev/null || true
            systemctl restart "${UNIT_NAME}" 2>/dev/null || true
            ;;
        openrc)
            rc-update add "${UNIT_NAME}" default 2>/dev/null || true
            rc-service "${UNIT_NAME}" start 2>/dev/null || true
            ;;
        *)
            nohup "${BINARY_PATH}" >/var/log/edgelet/daemon.log 2>&1 &
            ;;
    esac
}

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
    esac

    if [ -f packaging/systemd/edgelet.service ] && [ "$_full" = "true" ] && [ "$_eng" = "edgelet" ]; then
        cp packaging/systemd/edgelet.service "/etc/systemd/system/${UNIT_NAME}.service"
    else
        cat >"/etc/systemd/system/${UNIT_NAME}.service" <<EOF
[Unit]
Description=Edgelet daemon
Documentation=https://github.com/datasance/edgelet
${_wants}
After=${_after}
StartLimitIntervalSec=300
StartLimitBurst=20

[Service]
Type=simple
ExecStart=${BINARY_PATH}
Restart=always
RestartSec=2s
TimeoutStopSec=120s
KillMode=control-group
StandardOutput=journal
StandardError=journal
SyslogIdentifier=edgelet
LimitNOFILE=65536
NoNewPrivileges=$([ "$_full" = true ] && echo no || echo true)
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=${CONFIG_DIR} /var/lib/edgelet /var/lib/edgelet-containerd /var/log/edgelet /run/edgelet ${BACKUP_DIR}

[Install]
WantedBy=multi-user.target
EOF
    fi
    systemctl daemon-reload
    systemctl enable "${UNIT_NAME}"
    systemctl restart "${UNIT_NAME}"
    info "systemd unit ${UNIT_NAME}.service installed and started."
}

install_service() {
    _is_full="$1"
    case "${INIT}" in
        systemd) install_systemd "$_is_full" "$CONTAINER_ENGINE" ;;
        *)
            info "Non-systemd init: starting edgelet in background."
            start_edgelet_service
            ;;
    esac
}

compute_source_url() {
    if [ "$AIRGAP" = true ]; then
        _real="$(cd "$(dirname "$TARBALL_PATH")" && pwd)/$(basename "$TARBALL_PATH")"
        echo "file://${_real}"
    else
        echo "https://github.com/${GITHUB_REPO}/releases/download/${EDGELET_VERSION}/${TARBALL_BASENAME}"
    fi
}

maybe_provision() {
    if [ -z "$PROVISION_KEY" ]; then
        return 0
    fi
    if ! command -v edgelet >/dev/null 2>&1; then
        die "edgelet binary not found for provision"
    fi
    info "Running edgelet provision..."
    if [ "$NON_INTERACTIVE" = true ]; then
        edgelet --quiet provision "$PROVISION_KEY" || die "provision failed"
    else
        edgelet provision "$PROVISION_KEY" || die "provision failed"
    fi
}

# ── rollback ──────────────────────────────────────────────────────────────────
if [ "$ACTION" = "rollback" ]; then
    [ -f "$PREVIOUS_FILE" ] || die "No ${PREVIOUS_FILE} found."
    _pv=$(kv_get "$PREVIOUS_FILE" "previous_version")
    _pf=$(kv_get "$PREVIOUS_FILE" "previous_flavor")
    _purl=$(kv_get "$PREVIOUS_FILE" "previous_download_url")
    _cfgbak=$(kv_get "$PREVIOUS_FILE" "config_backup_path")
    EDGELET_FLAVOR="$_pf"
    EDGELET_VERSION="$_pv"
    TARBALL_BASENAME=$(tarball_name_for "$EDGELET_VERSION" "$ARCH" "$EDGELET_FLAVOR")
    CONTAINER_ENGINE="edgelet"
    [ "$EDGELET_FLAVOR" = "lite" ] && CONTAINER_ENGINE="docker"
    stop_edgelet_service
    _tg="${TMPDIR}/rollback.tar.gz"
    if [ "$AIRGAP" = true ]; then
        [ -n "$TARBALL_PATH" ] || die "rollback with --airgap requires --tarball-path"
        verify_tarball_checksum "$TARBALL_PATH"
        cp "$TARBALL_PATH" "$_tg"
    else
        curl -fsSL -o "$_tg" "$_purl" || die "Failed to download rollback tarball"
    fi
    extract_tarball "$_tg"
    install_binary
    install_dirs
    if [ "$FORCE_CONFIG" != true ] && [ -f "$_cfgbak" ]; then
        install -m 640 "$_cfgbak" "$CONFIG_FILE"
    fi
    _is_full="false"
    [ "$EDGELET_FLAVOR" = "full" ] && _is_full="true"
    install_service "$_is_full"
    write_install_receipt "$EDGELET_VERSION" "$EDGELET_FLAVOR" "$_purl"
    info "Rollback to ${EDGELET_VERSION} (${EDGELET_FLAVOR}) complete."
    exit 0
fi

# ── upgrade ───────────────────────────────────────────────────────────────────
if [ "$ACTION" = "upgrade" ]; then
    [ -f "$BINARY_PATH" ] || die "Edgelet not installed; run install first"
    [ -f "$RECEIPT_FILE" ] || die "Missing ${RECEIPT_FILE}"
    _cur_ver=$(kv_get "$RECEIPT_FILE" "installed_version")
    _cur_fl=$(kv_get "$RECEIPT_FILE" "flavor")
    _cur_src=$(kv_get "$RECEIPT_FILE" "source_url")
    [ "$_cur_fl" = "$EDGELET_FLAVOR" ] || die "Flavor mismatch: installed ${_cur_fl}, requested ${EDGELET_FLAVOR}"
    if [ "$EDGELET_VERSION" = "latest" ]; then
        EDGELET_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    fi
    TARBALL_BASENAME=$(tarball_name_for "$EDGELET_VERSION" "$ARCH" "$EDGELET_FLAVOR")
    _cfg_backup="${BACKUP_DIR}/config.yaml.$(date +%Y%m%d%H%M%S)"
    cp "$CONFIG_FILE" "$_cfg_backup" 2>/dev/null || true
    write_previous_release "$_cur_ver" "$_cur_fl" "$_cur_src" "$_cfg_backup"
    stop_edgelet_service
    _tg="${TMPDIR}/upgrade.tar.gz"
    download_or_copy_tarball "$_tg"
    extract_tarball "$_tg"
    install_binary
    install_dirs
    if [ "$FORCE_CONFIG" = true ]; then
        rm -f "$CONFIG_FILE"
        write_default_config_if_missing
    fi
    write_install_receipt "$EDGELET_VERSION" "$EDGELET_FLAVOR" "$(compute_source_url)"
    _is_full="false"
    [ "$EDGELET_FLAVOR" = "full" ] && _is_full="true"
    install_service "$_is_full"
    maybe_provision
    info "Upgrade to ${EDGELET_VERSION} complete."
    exit 0
fi

# ── fresh install ─────────────────────────────────────────────────────────────
_tg="${TMPDIR}/${TARBALL_BASENAME}"
download_or_copy_tarball "$_tg"
extract_tarball "$_tg"
install_dirs
install_binary

if [ -f "${TMPDIR}/config.yaml.sample" ] && [ ! -f "$CONFIG_FILE" ]; then
    install -m 640 "${TMPDIR}/config.yaml.sample" "$CONFIG_FILE"
    info "Sample config installed from tarball"
else
    write_default_config_if_missing
fi

if command -v sed >/dev/null 2>&1 && [ -f "$CONFIG_FILE" ]; then
    sed -i "s|containerEngine:.*|containerEngine: ${CONTAINER_ENGINE}|" "$CONFIG_FILE" 2>/dev/null || true
    _durl=$(default_docker_url_for_engine "$CONTAINER_ENGINE")
    sed -i "s|dockerUrl:.*|dockerUrl: ${_durl}|" "$CONFIG_FILE" 2>/dev/null || true
    if [ -n "$CONTROLLER_URL" ]; then
        sed -i "s|controllerUrl:.*|controllerUrl: \"${CONTROLLER_URL}\"|" "$CONFIG_FILE" 2>/dev/null || true
    fi
fi

write_install_receipt "$EDGELET_VERSION" "$EDGELET_FLAVOR" "$(compute_source_url)"
_is_full="false"
[ "$EDGELET_FLAVOR" = "full" ] && _is_full="true"
install_service "$_is_full"
install_cli_completion
maybe_provision

info ""
info "edgelet ${EDGELET_VERSION} (${EDGELET_FLAVOR}) installed successfully."
info "  Binary : ${BINARY_PATH}"
info "  Unit   : ${UNIT_NAME}.service"
info "  Config : ${CONFIG_FILE}"
info "  Data   : /var/lib/edgelet/"
info ""
info "Check status: edgelet system status"
info "Provision:    edgelet provision <key>"
