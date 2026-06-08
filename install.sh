#!/bin/sh
# install.sh — Edgelet greenfield installer (binary-only, multi-OS)
#
# Usage:
#   curl -fsSL .../install.sh | sudo sh -s -- --version=vX.Y.Z
#   sudo ./install.sh --bin-path=build/edgelet-linux-amd64 --version=dev
#   sudo ./install.sh --airgap --bin-path=./edgelet-linux-amd64 --expected-sha256=...
#
# Provision after install: edgelet provision <key>  (not install.sh flags)

set -e

die() { echo "ERROR: $1" >&2; exit 1; }
info() { echo ">>> $1"; }

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
if [ -f "${SCRIPT_DIR}/scripts/lib/init-detect.sh" ]; then
    LIB_DIR="${SCRIPT_DIR}/scripts/lib"
elif [ -f "${SCRIPT_DIR}/lib/init-detect.sh" ]; then
    LIB_DIR="${SCRIPT_DIR}/lib"
elif [ -f /usr/share/edgelet/lib/init-detect.sh ]; then
    LIB_DIR="/usr/share/edgelet/lib"
else
    die "Missing init helper scripts (scripts/lib, lib/, or /usr/share/edgelet/lib)"
fi
# shellcheck source=scripts/lib/init-detect.sh
. "${LIB_DIR}/init-detect.sh"
# shellcheck source=scripts/lib/init-edgelet.sh
. "${LIB_DIR}/init-edgelet.sh"

BACKUP_DIR="/var/backups/edgelet"
CACHE_DIR="${BACKUP_DIR}/cache"
RECEIPT_FILE="${BACKUP_DIR}/install-receipt"
PREVIOUS_FILE="${BACKUP_DIR}/previous-release"
GITHUB_REPO="${EDGELET_GITHUB_REPO:-datasance/edgelet}"
SHARE_DIR="/usr/share/edgelet"
UNIT_NAME="edgelet"
CONFIG_DIR="/etc/edgelet"
CONFIG_FILE="${CONFIG_DIR}/config.yaml"
CERT_FILE="${CONFIG_DIR}/cert.crt"

detect_os() {
    _u=$(uname -s)
    case "${_u}" in
        Linux)  echo "linux" ;;
        Darwin) echo "darwin" ;;
        MINGW*|MSYS*|CYGWIN*|Windows_NT) echo "windows" ;;
        *) die "Unsupported OS: ${_u}" ;;
    esac
}

detect_arch() {
    MACHINE=$(uname -m)
    case "${MACHINE}" in
        x86_64|amd64)     echo "amd64" ;;
        aarch64|arm64)    echo "arm64" ;;
        armv7l|armv6l|arm) echo "arm" ;;
        riscv64)          echo "riscv64" ;;
        *) die "Unsupported architecture: ${MACHINE}" ;;
    esac
}

require_root() {
    OS=$(detect_os)
    if [ "$OS" = "windows" ]; then
        return 0
    fi
    [ "$(id -u)" -eq 0 ] || die "This script must be run as root. Try: sudo $0 $*"
}

binary_basename() {
    _os="$1" _arch="$2"
    case "${_os}" in
        windows) echo "edgelet-${_os}-${_arch}.exe" ;;
        *)       echo "edgelet-${_os}-${_arch}" ;;
    esac
}

binary_install_path() {
    _os="$1"
    case "${_os}" in
        linux|darwin) echo "/usr/local/bin/edgelet" ;;
        windows)
            _pf="${ProgramFiles:-/c/Program Files}"
            echo "${_pf}/Edgelet/edgelet.exe"
            ;;
        *) die "Unsupported OS for binary path" ;;
    esac
}

release_download_url() {
    _ver="$1" _os="$2" _arch="$3"
    _base="https://github.com/${GITHUB_REPO}/releases/download/${_ver}/$(binary_basename "$_os" "$_arch")"
    echo "$_base"
}

verify_binary_checksum() {
    _bin="$1"
    [ -f "$_bin" ] || die "Not a file: $_bin"
    if [ -n "$EXPECTED_SHA256" ]; then
        _sum=$(sha256sum "$_bin" | awk '{print $1}')
        [ "$_sum" = "$EXPECTED_SHA256" ] || die "SHA256 mismatch (expected $EXPECTED_SHA256 got $_sum)"
        info "SHA256 verified."
    elif [ -n "$CHECKSUM_FILE" ] && [ -f "$CHECKSUM_FILE" ]; then
        _bn=$(basename "$_bin")
        ( cd "$(dirname "$_bin")" && grep " ${_bn}\$" "$CHECKSUM_FILE" >/dev/null ) || \
            ( cd "$(dirname "$CHECKSUM_FILE")" && sha256sum -c "$CHECKSUM_FILE" ) || \
            die "Checksum file verification failed"
    fi
}

sha256_file() {
    sha256sum "$1" | awk '{print $1}'
}

kv_get() {
    _file="$1" _key="$2"
    [ -f "$_file" ] || { echo ""; return 0; }
    _line=$(grep "^${_key}=" "$_file" | head -1) || true
    [ -n "$_line" ] || { echo ""; return 0; }
    echo "$_line" | sed "s/^${_key}=//"
}

write_install_receipt() {
    _ver="$1" _os="$2" _arch="$3" _eng="$4" _url="$5" _sha="$6" _method="$7"
    mkdir -p "$BACKUP_DIR"
    _ts=$(date -u '+%Y-%m-%dT%H:%M:%SZ' 2>/dev/null || date -u)
    {
        printf 'installed_version=%s\n' "$_ver"
        printf 'os=%s\n' "$_os"
        printf 'arch=%s\n' "$_arch"
        printf 'container_engine=%s\n' "$_eng"
        printf 'source_url=%s\n' "$_url"
        printf 'installed_at=%s\n' "$_ts"
        printf 'install_method=%s\n' "$_method"
        printf 'binary_sha256=%s\n' "$_sha"
    } >"$RECEIPT_FILE"
    chmod 600 "$RECEIPT_FILE" 2>/dev/null || true
}

write_previous_release() {
    _pv="$1" _pos="$2" _parch="$3" _peng="$4" _purl="$5" _psha="$6" _cfg="$7"
    mkdir -p "$BACKUP_DIR"
    {
        printf 'previous_version=%s\n' "$_pv"
        printf 'previous_os=%s\n' "$_pos"
        printf 'previous_arch=%s\n' "$_parch"
        printf 'previous_container_engine=%s\n' "$_peng"
        printf 'previous_download_url=%s\n' "$_purl"
        printf 'previous_binary_sha256=%s\n' "$_psha"
        printf 'config_backup_path=%s\n' "$_cfg"
    } >"$PREVIOUS_FILE"
    chmod 600 "$PREVIOUS_FILE" 2>/dev/null || true
}

cache_binary() {
    _ver="$1" _os="$2" _arch="$3" _src="$4"
    mkdir -p "$CACHE_DIR"
    _dest="${CACHE_DIR}/edgelet-${_ver}-${_os}-${_arch}"
    case "${_os}" in
        windows) _dest="${_dest}.exe" ;;
    esac
    cp "$_src" "$_dest"
    chmod 755 "$_dest" 2>/dev/null || true
    info "Cached binary at ${_dest}"
}

cached_binary_path() {
    _ver="$1" _os="$2" _arch="$3"
    _p="${CACHE_DIR}/edgelet-${_ver}-${_os}-${_arch}"
    case "${_os}" in
        windows) _p="${_p}.exe" ;;
    esac
    if [ -f "$_p" ]; then
        echo "$_p"
        return 0
    fi
    echo ""
}

packaging_etc_dir() {
    if [ -d "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet" ]; then
        echo "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet"
        return 0
    fi
    if [ -d "${SHARE_DIR}/etc/edgelet" ]; then
        echo "${SHARE_DIR}/etc/edgelet"
        return 0
    fi
    echo ""
}

install_config_samples() {
    _etc="$(packaging_etc_dir)"
    _sample_cfg=""
    _sample_ca=""
    if [ -n "$_etc" ]; then
        [ -f "${_etc}/config.default.yaml" ] && _sample_cfg="${_etc}/config.default.yaml"
        [ -f "${_etc}/controller-ca.sample.crt" ] && _sample_ca="${_etc}/controller-ca.sample.crt"
    fi
    if [ -f "${SHARE_DIR}/edgelet-config.yaml.sample" ]; then
        _sample_cfg="${SHARE_DIR}/edgelet-config.yaml.sample"
    fi
    if [ -f "${SHARE_DIR}/edgelet-controller-ca.crt.sample" ]; then
        _sample_ca="${SHARE_DIR}/edgelet-controller-ca.crt.sample"
    fi

    if [ ! -f "$CONFIG_FILE" ]; then
        if [ -n "$_sample_cfg" ]; then
            mkdir -p "$CONFIG_DIR"
            install -m 640 "$_sample_cfg" "$CONFIG_FILE"
            info "Config installed from sample."
        else
            write_default_config_if_missing
        fi
    elif [ "$FORCE_CONFIG" = true ]; then
        [ -n "$_sample_cfg" ] || die "--force-config requires a config sample"
        install -m 640 "$_sample_cfg" "$CONFIG_FILE"
        info "Config replaced (--force-config)."
    else
        info "Existing config preserved at ${CONFIG_FILE}"
    fi

    if [ "$WITH_SAMPLE_CA" = true ] && [ ! -f "$CERT_FILE" ]; then
        [ -n "$_sample_ca" ] || die "--with-sample-ca requires controller-ca sample"
        install -m 644 "$_sample_ca" "$CERT_FILE"
        info "Sample controller CA installed at ${CERT_FILE}"
    fi
}

write_default_config_if_missing() {
    [ -f "$CONFIG_FILE" ] && return 0
    _eng="${CONTAINER_ENGINE:-edgelet}"
    _du=$(default_container_engine_url_for_engine "$_eng")
    mkdir -p "$CONFIG_DIR"
    cat >"$CONFIG_FILE" <<YAML
currentProfile: production
profiles:
  production:
    controllerUrl: "http://localhost:54421/api/v3/"
    controllerCert: "${CERT_FILE}"
    arch: auto
    containerEngine: ${_eng}
    containerEngineUrl: ${_du}
    diskDirectory: /var/lib/edgelet/
    logDiskDirectory: /var/log/edgelet/
    logLevel: INFO
YAML
    chmod 640 "$CONFIG_FILE"
    info "Default config installed at ${CONFIG_FILE}"
}

default_container_engine_url_for_engine() {
    case "$1" in
        docker) echo "unix:///var/run/docker.sock" ;;
        podman) echo "unix:///run/podman/podman.sock" ;;
        edgelet) echo "unix:///run/edgelet/containerd.sock" ;;
        *) die "Unsupported engine: $1" ;;
    esac
}

install_dirs() {
    OS="$1"
    case "${OS}" in
        linux|darwin)
            mkdir -p "$CONFIG_DIR" /var/log/edgelet /var/lib/edgelet /run/edgelet \
                /var/lib/edgelet-containerd "$BACKUP_DIR" "$CACHE_DIR" "$SHARE_DIR"
            chmod 750 "$CONFIG_DIR" /var/log/edgelet /var/lib/edgelet 2>/dev/null || true
            ;;
        windows)
            _pd="${ProgramData:-/c/ProgramData}/Edgelet"
            mkdir -p "${_pd}/data" "${_pd}/config" 2>/dev/null || true
            ;;
    esac
}

copy_bundled_scripts() {
    OS="$1"
    [ "$OS" = "linux" ] || return 0
    mkdir -p "$SHARE_DIR"
    install -m 755 "${SCRIPT_DIR}/install.sh" "${SHARE_DIR}/install.sh"
    install -m 755 "${SCRIPT_DIR}/uninstall.sh" "${SHARE_DIR}/uninstall.sh"
    if [ -d "${SCRIPT_DIR}/packaging/init" ]; then
        rm -rf "${SHARE_DIR}/init"
        cp -R "${SCRIPT_DIR}/packaging/init" "${SHARE_DIR}/init"
    fi
    if [ -d "${SCRIPT_DIR}/scripts/lib" ]; then
        mkdir -p "${SHARE_DIR}/lib"
        cp "${SCRIPT_DIR}/scripts/lib/"*.sh "${SHARE_DIR}/lib/"
        chmod 755 "${SHARE_DIR}/lib/"*.sh
    fi
    if [ -f "${SCRIPT_DIR}/scripts/edgelet-shutdown" ]; then
        install -m 755 "${SCRIPT_DIR}/scripts/edgelet-shutdown" "${SHARE_DIR}/edgelet-shutdown"
    fi
    if [ -d "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet" ]; then
        mkdir -p "${SHARE_DIR}/etc/edgelet"
        for _f in config.default.yaml controller-ca.sample.crt; do
            [ -f "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet/${_f}" ] && \
                cp "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet/${_f}" "${SHARE_DIR}/etc/edgelet/${_f}" 2>/dev/null || true
        done
        [ -f "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet/config.default.yaml" ] && \
            cp "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet/config.default.yaml" \
                "${SHARE_DIR}/edgelet-config.yaml.sample"
        [ -f "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet/controller-ca.sample.crt" ] && \
            cp "${SCRIPT_DIR}/packaging/edgelet/etc/edgelet/controller-ca.sample.crt" \
                "${SHARE_DIR}/edgelet-controller-ca.crt.sample"
    fi
    info "Bundled install scripts at ${SHARE_DIR}/"
}

install_binary_file() {
    _src="$1" _dest="$2"
    _dir=$(dirname "$_dest")
    mkdir -p "$_dir"
    install -m 755 "$_src" "$_dest"
    info "Installed ${_dest}"
}

install_cli_completion() {
    command -v edgelet >/dev/null 2>&1 || return 0
    if [ -d /etc/bash_completion.d ]; then
        if edgelet completion bash >/etc/bash_completion.d/edgelet 2>/dev/null; then
            chmod 644 /etc/bash_completion.d/edgelet
            info "Bash completion installed."
        fi
    fi
}

apply_container_engine_to_config() {
    [ -f "$CONFIG_FILE" ] || return 0
    command -v sed >/dev/null 2>&1 || return 0
    sed -i "s|containerEngine:.*|containerEngine: ${CONTAINER_ENGINE}|" "$CONFIG_FILE" 2>/dev/null || true
    _durl=$(default_container_engine_url_for_engine "$CONTAINER_ENGINE")
    sed -i "s|containerEngineUrl:.*|containerEngineUrl: ${_durl}|" "$CONFIG_FILE" 2>/dev/null || true
}

download_or_stage_binary() {
    _dest="$1"
    if [ -n "$BIN_PATH" ]; then
        [ -f "$BIN_PATH" ] || die "Local binary not found: $BIN_PATH"
        verify_binary_checksum "$BIN_PATH"
        cp "$BIN_PATH" "$_dest"
        info "Using local binary: ${BIN_PATH}"
        return 0
    fi
    if [ "$AIRGAP" = true ]; then
        die "--airgap requires --bin-path"
    fi
    _url=$(release_download_url "$EDGELET_VERSION" "$OS" "$ARCH")
    info "Downloading ${_url} ..."
    curl -fsSL -o "$_dest" "$_url" || die "Failed to download release binary"
}

compute_source_url() {
    if [ -n "$BIN_PATH" ]; then
        _real=$(cd "$(dirname "$BIN_PATH")" && pwd)/$(basename "$BIN_PATH")
        echo "file://${_real}"
    else
        release_download_url "$EDGELET_VERSION" "$OS" "$ARCH"
    fi
}

# ── argument parsing ──────────────────────────────────────────────────────────
EDGELET_VERSION="${EDGELET_VERSION:-latest}"
CONTAINER_ENGINE=""
ACTION="install"
AIRGAP=false
BIN_PATH=""
FORCE_CONFIG=false
WITH_SAMPLE_CA=false
ARCH_OVERRIDE=""
CHECKSUM_FILE=""
EXPECTED_SHA256=""

for arg in "$@"; do
    case "${arg}" in
        --version=*)          EDGELET_VERSION="${arg#*=}" ;;
        --arch=*)             ARCH_OVERRIDE="${arg#*=}" ;;
        --container-engine=*) CONTAINER_ENGINE="${arg#*=}" ;;
        --bin-path=*)         BIN_PATH="${arg#*=}" ;;
        --checksum-path=*)    CHECKSUM_FILE="${arg#*=}" ;;
        --expected-sha256=*)  EXPECTED_SHA256="${arg#*=}" ;;
        --airgap)             AIRGAP=true ;;
        --upgrade)            ACTION="upgrade" ;;
        --rollback)           ACTION="rollback" ;;
        --force-config)       FORCE_CONFIG=true ;;
        --with-sample-ca)     WITH_SAMPLE_CA=true ;;
        --help|-h)
            cat <<EOF
Usage: $0 [options]

Options:
  --version=VERSION          Release tag (default: latest)
  --arch=ARCH                Override arch (amd64, arm64, arm, riscv64)
  --container-engine=ENGINE  edgelet, docker, or podman (linux default: edgelet)
  --airgap                   Do not download; use --bin-path
  --bin-path=PATH            Local edgelet binary
  --checksum-path=PATH       SHA256SUMS manifest for verification
  --expected-sha256=HASH     Verify local binary SHA256
  --upgrade / --rollback     In-place thin binary OTA
  --force-config             Replace config from sample
  --with-sample-ca           Install sample controller CA if cert missing

Environment:
  EDGELET_VERSION  EDGELET_GITHUB_REPO
EOF
            exit 0 ;;
        *) die "Unknown option: ${arg} (use --help)" ;;
    esac
done

if [ "$AIRGAP" = true ] && [ -z "$BIN_PATH" ]; then
    die "--airgap requires --bin-path"
fi

OS=$(detect_os)
ARCH="${ARCH_OVERRIDE:-$(detect_arch)}"
BINARY_PATH=$(binary_install_path "$OS")

if [ "$OS" = "linux" ]; then
    INIT=$(detect_init)
else
    INIT="none"
fi

if [ "$ACTION" != "rollback" ]; then
    if [ -z "$CONTAINER_ENGINE" ]; then
        case "$OS" in
            linux) CONTAINER_ENGINE="edgelet" ;;
            *)     CONTAINER_ENGINE="docker" ;;
        esac
    fi
    case "$CONTAINER_ENGINE" in
        edgelet)
            [ "$OS" = "linux" ] || die "containerEngine=edgelet is linux-only"
            ;;
        docker|podman) ;;
        *) die "Invalid --container-engine (use edgelet, docker, or podman)" ;;
    esac
fi

require_root

info "OS           : ${OS}"
info "Architecture : ${ARCH}"
info "Init system  : ${INIT}"
if [ "$ACTION" != "rollback" ]; then
    info "Engine       : ${CONTAINER_ENGINE}"
fi
info "Action       : ${ACTION}"

TMPDIR=$(mktemp -d)
trap 'rm -rf "${TMPDIR}"' EXIT

if [ "$AIRGAP" = false ] && [ "$EDGELET_VERSION" = "latest" ] && [ "$ACTION" != "rollback" ] && [ -z "$BIN_PATH" ]; then
    info "Fetching latest release tag..."
    EDGELET_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
        | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    [ -n "${EDGELET_VERSION}" ] || die "Failed to determine latest version"
fi

info "Version: ${EDGELET_VERSION}"

# ── rollback ──────────────────────────────────────────────────────────────────
if [ "$ACTION" = "rollback" ]; then
    [ -f "$PREVIOUS_FILE" ] || die "No ${PREVIOUS_FILE} found."
    _pv=$(kv_get "$PREVIOUS_FILE" "previous_version")
    _pos=$(kv_get "$PREVIOUS_FILE" "previous_os")
    _parch=$(kv_get "$PREVIOUS_FILE" "previous_arch")
    _peng=$(kv_get "$PREVIOUS_FILE" "previous_container_engine")
    _purl=$(kv_get "$PREVIOUS_FILE" "previous_download_url")
    _cfgbak=$(kv_get "$PREVIOUS_FILE" "config_backup_path")
    [ -n "$_pos" ] || _pos="$OS"
    [ -n "$_parch" ] || _parch="$ARCH"
    CONTAINER_ENGINE="${_peng:-edgelet}"
    EDGELET_VERSION="$_pv"
    _staged="${TMPDIR}/edgelet-bin"
    _cached=$(cached_binary_path "$_pv" "$_pos" "$_parch")
    if [ -n "$_cached" ]; then
        cp "$_cached" "$_staged"
        info "Rollback from cache: ${_cached}"
    elif [ -n "$BIN_PATH" ]; then
        verify_binary_checksum "$BIN_PATH"
        cp "$BIN_PATH" "$_staged"
    elif [ "$AIRGAP" = true ]; then
        die "rollback with --airgap requires --bin-path or a cached binary"
    else
        curl -fsSL -o "$_staged" "$_purl" || die "Failed to download rollback binary"
    fi
    [ "$OS" = "linux" ] && stop_edgelet_service "$INIT"
    install_binary_file "$_staged" "$BINARY_PATH"
    install_dirs "$OS"
    if [ "$FORCE_CONFIG" != true ] && [ -f "$_cfgbak" ]; then
        install -m 640 "$_cfgbak" "$CONFIG_FILE"
    fi
    if [ "$OS" = "linux" ]; then
        install_init_unit "$INIT" "$CONTAINER_ENGINE"
    fi
    _sha=$(sha256_file "$BINARY_PATH")
    write_install_receipt "$EDGELET_VERSION" "$_pos" "$_parch" "$CONTAINER_ENGINE" "$_purl" "$_sha" "rollback"
    info "Rollback to ${EDGELET_VERSION} complete."
    exit 0
fi

# ── upgrade ───────────────────────────────────────────────────────────────────
if [ "$ACTION" = "upgrade" ]; then
    [ -f "$BINARY_PATH" ] || die "Edgelet not installed; run install first"
    [ -f "$RECEIPT_FILE" ] || die "Missing ${RECEIPT_FILE}"
    _cur_ver=$(kv_get "$RECEIPT_FILE" "installed_version")
    _cur_os=$(kv_get "$RECEIPT_FILE" "os")
    _cur_arch=$(kv_get "$RECEIPT_FILE" "arch")
    _cur_eng=$(kv_get "$RECEIPT_FILE" "container_engine")
    _cur_src=$(kv_get "$RECEIPT_FILE" "source_url")
    _cur_sha=$(kv_get "$RECEIPT_FILE" "binary_sha256")
    [ -n "$_cur_os" ] || _cur_os="$OS"
    [ -n "$_cur_arch" ] || _cur_arch="$ARCH"
    [ -n "$_cur_eng" ] || _cur_eng="$CONTAINER_ENGINE"
    if [ "$EDGELET_VERSION" = "latest" ] && [ -z "$BIN_PATH" ]; then
        EDGELET_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" \
            | grep '"tag_name"' | head -1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
    fi
    _cfg_backup="${BACKUP_DIR}/config.yaml.$(date +%Y%m%d%H%M%S 2>/dev/null || date +%s)"
    cp "$CONFIG_FILE" "$_cfg_backup" 2>/dev/null || true
    cache_binary "$_cur_ver" "$_cur_os" "$_cur_arch" "$BINARY_PATH"
    write_previous_release "$_cur_ver" "$_cur_os" "$_cur_arch" "$_cur_eng" "$_cur_src" "$_cur_sha" "$_cfg_backup"
    [ "$OS" = "linux" ] && stop_edgelet_service "$INIT"
    _staged="${TMPDIR}/edgelet-bin"
    download_or_stage_binary "$_staged"
    verify_binary_checksum "$_staged"
    install_binary_file "$_staged" "$BINARY_PATH"
    install_dirs "$OS"
    if [ "$FORCE_CONFIG" = true ]; then
        rm -f "$CONFIG_FILE"
        install_config_samples
    fi
    apply_container_engine_to_config
    _sha=$(sha256_file "$BINARY_PATH")
    _method="upgrade"
    [ "$AIRGAP" = true ] && _method="upgrade-airgap"
    write_install_receipt "$EDGELET_VERSION" "$OS" "$ARCH" "$CONTAINER_ENGINE" "$(compute_source_url)" "$_sha" "$_method"
    copy_bundled_scripts "$OS"
    if [ "$OS" = "linux" ]; then
        install_init_unit "$INIT" "$CONTAINER_ENGINE"
    fi
    info "Upgrade to ${EDGELET_VERSION} complete."
    exit 0
fi

# ── fresh install ─────────────────────────────────────────────────────────────
_staged="${TMPDIR}/edgelet-bin"
download_or_stage_binary "$_staged"
verify_binary_checksum "$_staged"
install_dirs "$OS"
install_binary_file "$_staged" "$BINARY_PATH"
install_config_samples
apply_container_engine_to_config
_sha=$(sha256_file "$BINARY_PATH")
_method="install"
[ "$AIRGAP" = true ] && _method="install-airgap"
write_install_receipt "$EDGELET_VERSION" "$OS" "$ARCH" "$CONTAINER_ENGINE" "$(compute_source_url)" "$_sha" "$_method"
copy_bundled_scripts "$OS"
if [ "$OS" = "linux" ]; then
    install_init_unit "$INIT" "$CONTAINER_ENGINE"
    install_cli_completion
fi

info ""
info "edgelet ${EDGELET_VERSION} installed (os=${OS} engine=${CONTAINER_ENGINE})."
info "  Binary : ${BINARY_PATH}"
if [ "$OS" = "linux" ]; then
    info "  Unit   : ${INIT}"
    info "  Config : ${CONFIG_FILE}"
    info "  Data   : /var/lib/edgelet/"
fi
info ""
info "Check status: edgelet system status"
info "Configure Controller: edgelet config --a <controller-api-endpoint>"
info "Provision:    edgelet provision <key>"
