#!/usr/bin/env bash
# test/lima/lib/remote.sh — Lima VM remote helpers. Source only.

# lima_repo_root — repository root from caller script location.
lima_repo_root() {
    cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd
}

# lima_status VM — Running|Stopped|… or empty if missing.
lima_status() {
    local _vm="$1"
    limactl list --json 2>/dev/null \
        | jq -r "select(.name == \"${_vm}\") | .status" 2>/dev/null \
        | head -1 || true
}

# lima_remote VM 'script' — pipe bash to sudo in VM (Lima --tty=false).
lima_remote() {
    local _vm="$1"
    shift
    echo "$*" | limactl --tty=false shell "${_vm}" -- sudo bash
}

# lima_ssh_config VM — path to Lima SSH config.
lima_ssh_config() {
    echo "${HOME}/.lima/${1}/ssh.config"
}

# lima_ssh_host VM — SSH host alias from Lima config.
lima_ssh_host() {
    echo "lima-${1}"
}
