#!/usr/bin/env bash
# Run make test-unit inside Linux Docker (macOS-friendly parity with ci.yml Test job).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

GO_IMAGE="${GO_IMAGE:-golang:1.26.4}"
GOARCH="${GOARCH:-$(go env GOARCH)}"
TEST_FLAGS="${TEST_FLAGS:-}"

echo "=== test-linux (linux/${GOARCH}, same as CI make test-unit) ==="

docker run --rm --platform "linux/${GOARCH}" \
	-e "TEST_FLAGS=${TEST_FLAGS}" \
	-v "${ROOT}:/src" -w /src "${GO_IMAGE}" bash -euxo pipefail -c "
		go version
		go mod download
		export GO111MODULE=on
		unset GOPATH
		_TEST_PKGS='./cmd/... ./internal/... ./pkg/... ./test/...'
		go test \${TEST_FLAGS} -v -short -tags '!linux' \${_TEST_PKGS}
		go test \${TEST_FLAGS} -v -short -tags linux \${_TEST_PKGS}
		CGO_ENABLED=1 go test \${TEST_FLAGS} -v -short -tags 'linux,cgo' \${_TEST_PKGS}
	"

echo "test-linux (linux/${GOARCH}): passed (test-unit parity)"
