#!/bin/bash
# Build the thin edgelet-kube binary and the fat runtime it extracts.
# Thin is CGO_ENABLED=0. Fat is CGO_ENABLED=1.
# The zstd bundle holds the fat runtime and the aux binaries. It holds no OCI image.
set -euo pipefail

cd "$(dirname "$0")/.."
export GOWORK=off
arch="${ARCH:-amd64}"
root="$(cd .. && pwd)"

bash scripts/bundle-stage.sh ensure "$root" "$arch"

mkdir -p build
# A non-Linux host needs a Linux C compiler for the fat runtime.
if [ "$(uname -s)" != "Linux" ] && [ "$arch" = "amd64" ]; then
	CC="${CC:-x86_64-linux-gnu-gcc}" CGO_ENABLED=1 GOOS=linux GOARCH="$arch" \
		go build -trimpath -o build/edgelet-kube-server ./cmd/edgelet-kube-server
else
	CGO_ENABLED=1 GOOS=linux GOARCH="$arch" \
		go build -trimpath -o build/edgelet-kube-server ./cmd/edgelet-kube-server
fi

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
bash scripts/bundle-stage.sh assemble "$stage" "$root" build/edgelet-kube-server
env -u GOOS -u GOARCH go run ./scripts/pack -stage "$stage" -out cmd/edgelet-kube/embed
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o build/edgelet-kube ./cmd/edgelet-kube
