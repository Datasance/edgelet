#!/bin/bash
# Build the thin edgelet-kube binary and the fat runtime it extracts.
# Thin is CGO_ENABLED=0. Fat is CGO_ENABLED=1.
# The zstd bundle holds the fat runtime and the aux binaries. It holds no OCI image.
# The airgap image archive is published beside this binary. It is not embedded here.
set -euo pipefail

cd "$(dirname "$0")/.."
export GOWORK=off
arch="${ARCH:-amd64}"
root="$(cd .. && pwd)"

bash scripts/bundle-stage.sh ensure "$root" "$arch"

mkdir -p build
k8s_version=$(awk '$1 == "kubernetes" { print $2; exit }' scripts/airgap/pins.txt)
k8s_rest=${k8s_version#v}
k8s_major=${k8s_rest%%.*}
k8s_minor=${k8s_rest#*.}
k8s_minor=${k8s_minor%%.*}
version_ldflags="-X k8s.io/component-base/version.gitVersion=${k8s_version} -X k8s.io/component-base/version.gitMajor=${k8s_major} -X k8s.io/component-base/version.gitMinor=${k8s_minor} -X k8s.io/client-go/pkg/version.gitVersion=${k8s_version} -X k8s.io/client-go/pkg/version.gitMajor=${k8s_major} -X k8s.io/client-go/pkg/version.gitMinor=${k8s_minor}"
# A non-Linux host needs a Linux C compiler for the fat runtime.
if [ "$(uname -s)" != "Linux" ] && [ "$arch" = "amd64" ]; then
	CC="${CC:-x86_64-linux-gnu-gcc}" CGO_ENABLED=1 GOOS=linux GOARCH="$arch" \
		go build -trimpath -ldflags "${version_ldflags}" -o build/edgelet-kube-server ./cmd/edgelet-kube-server
else
	CGO_ENABLED=1 GOOS=linux GOARCH="$arch" \
		go build -trimpath -ldflags "${version_ldflags}" -o build/edgelet-kube-server ./cmd/edgelet-kube-server
fi

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
bash scripts/bundle-stage.sh assemble "$stage" "$root" build/edgelet-kube-server
env -u GOOS -u GOARCH go run ./scripts/pack -stage "$stage" -out cmd/edgelet-kube/embed
CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath -o build/edgelet-kube ./cmd/edgelet-kube
