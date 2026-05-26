#!/usr/bin/env bash
set -euo pipefail

echo "[gate] invariants"
go test ./internal/processmanager ./internal/fieldagent ./internal/config

echo "[gate] transport-auth"
go test ./internal/auth ./internal/localapi ./internal/serviceaccount

echo "[gate] storage"
go test ./internal/store

echo "[gate] runtime-api"
go test ./internal/localapi ./internal/runtimeapi

echo "[gate] cli"
go test ./internal/cli ./cmd/edgelet

echo "LocalAPI v3 QA gates passed."
