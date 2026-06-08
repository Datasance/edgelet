#!/bin/sh
# Plan 10 guard: single canonical init tree under packaging/init/.
set -e
ROOT="$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)"
cd "${ROOT}"

if [ -f packaging/systemd/edgelet.service ]; then
    echo "ERROR: duplicate packaging/systemd/edgelet.service — use packaging/init/systemd/ only" >&2
    exit 1
fi

if grep -R 'packaging/systemd/' scripts/lib/init-edgelet.sh install.sh uninstall.sh 2>/dev/null; then
    echo "ERROR: install scripts must not reference packaging/systemd/" >&2
    exit 1
fi

echo "check-init-packaging: OK"
