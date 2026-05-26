#!/bin/bash
# Edgelet embed pipeline version pins (RFC R16–R19, version-matrix.md).

VERSION_GOLANG="1.26.2"

GO=${GO-go}
ARCH=${ARCH:-$("${GO}" env GOARCH)}
OS=${OS:-$("${GO}" env GOOS)}

get-module-version() {
  "${GO}" list -mod=readonly -m -f '{{if .Replace}}{{.Replace.Version}}{{else}}{{.Version}}{{end}}' "$1"
}

get-module-path() {
  "${GO}" list -mod=readonly -m -f '{{if .Replace}}{{.Replace.Path}}{{else}}{{.Path}}{{end}}' "$1"
}

PKG_CONTAINERD=$(get-module-path github.com/containerd/containerd/v2)
VERSION_CONTAINERD=$(get-module-version github.com/containerd/containerd/v2)
if [ -z "${VERSION_CONTAINERD}" ]; then
  VERSION_CONTAINERD="v2.2.3-k3s1"
fi

VERSION_CNIPLUGINS="v1.9.1-k3s1"
VERSION_CRUN="1.27.1"
VERSION_PAUSE="portainer/pause:latest"

BINARY_POSTFIX=
if [ "${OS}" = windows ]; then
  BINARY_POSTFIX=.exe
fi
