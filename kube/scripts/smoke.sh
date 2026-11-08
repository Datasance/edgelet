#!/bin/bash
# Run the thin binary on linux/amd64: server exits 0, port 6443 stays closed,
# and kubectl, ctr, and crictl reach the fat runtime.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$(uname -s)" != "Linux" ] || [ "$(uname -m)" != "x86_64" ]; then
	echo "this check runs on linux/amd64" >&2
	exit 1
fi

thin="build/edgelet-kube"
fat="build/edgelet-kube-server"
if [ ! -x "$thin" ] || [ ! -x "$fat" ]; then
	echo "run scripts/build.sh first" >&2
	exit 1
fi

go version -m "$fat" | grep -q 'CGO_ENABLED=1'
go version -m "$thin" | grep -q 'CGO_ENABLED=0'

listen_6443() {
	awk 'NR > 1 && $4 == "0A" && $2 ~ /:192B$/' /proc/net/tcp /proc/net/tcp6 2>/dev/null || true
}

data=$(mktemp -d)
trap 'rm -rf "$data"' EXIT

out="$data/server.out"
if ! timeout 15s "$thin" server --data-dir "$data/work" >"$out"; then
	echo "server failed" >&2
	cat "$out" >&2
	exit 1
fi
grep -q 'edgelet-kube runtime server' "$out"
if listen_6443 | grep -q .; then
	echo "port 6443 is listening" >&2
	listen_6443 >&2
	exit 1
fi

mapfile -t extracted < <(find "$data/work/data" -type f -name edgelet-kube)
if [ "${#extracted[@]}" -ne 1 ]; then
	echo "expected one extracted runtime, found ${#extracted[@]}" >&2
	exit 1
fi
cmp -s "$fat" "${extracted[0]}"
[ -x "${extracted[0]}" ]

for name in kubectl ctr crictl; do
	link="$data/$name"
	ln -s "$(pwd)/$thin" "$link"
	got=$(timeout 15s "$link" --data-dir "$data/work")
	printf '%s\n' "$got" | grep -q "edgelet-kube runtime $name"
	sub=$(timeout 15s "$thin" "$name" --data-dir "$data/work")
	printf '%s\n' "$sub" | grep -q "edgelet-kube runtime $name"
done

empty="$data/empty"
mkdir -p "$empty"
if "$thin" --data-dir "$empty" >"$empty/out" 2>"$empty/err"; then
	echo "missing command should fail" >&2
	exit 1
fi
if [ -d "$empty/data" ]; then
	echo "a usage error extracted a bundle" >&2
	exit 1
fi
