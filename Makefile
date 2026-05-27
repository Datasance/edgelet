.PHONY: build build-cli build-daemon build-daemon-embedded build-edgelet build-edgelet-linux build-edgelet-local deps test lint lint-fix clean docker-build docker-build-dev install install-dev start-dev stop-dev setup-dev-env export-dev-env fmt vet help build-all-archs build-linux-amd64 build-linux-arm64 build-linux-arm build-linux-riscv64 release-tarballs build-desktop-darwin build-desktop-windows desktop-dev test-embedded test-embedded-ci cli-docs cli-docs-check cli-help-check cli-completion test-embedded-docker ci-docker

GOBIN ?= $(shell go env GOBIN)
ifeq ($(GOBIN),)
GOBIN := $(shell go env GOPATH)/bin
endif

export PATH := $(GOBIN):$(PATH)

# golangci-lint — pinned version; override with GOLANGCI_LINT_VERSION=vX.Y.Z
GOLANGCI_LINT_VERSION ?= v1.64.4
GOLANGCI_LINT         := $(GOBIN)/golangci-lint

# Version and build info
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GIT_COMMIT := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")

# Linux release arch (amd64, arm64, arm, riscv64)
ARCH ?= amd64

# Build flags — platform capability comes from GOOS (linux embed vs desktop monolithic).
LDFLAGS_EDGELET := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT) \
	-X github.com/datasance/edgelet/internal/cli/cmd.Version=$(VERSION) \
	-X github.com/datasance/edgelet/internal/cli/cmd.BuildTime=$(BUILD_TIME) \
	-X github.com/datasance/edgelet/internal/cli/cmd.GitCommit=$(GIT_COMMIT) -s -w
BUILD_FLAGS_EDGELET := -trimpath -ldflags "$(LDFLAGS_EDGELET)"

# Legacy aliases (CLI/daemon were separate binaries pre-Plan 3).
LDFLAGS_CLI := -X github.com/datasance/edgelet/internal/cli/cmd.Version=$(VERSION) -X github.com/datasance/edgelet/internal/cli/cmd.BuildTime=$(BUILD_TIME) -X github.com/datasance/edgelet/internal/cli/cmd.GitCommit=$(GIT_COMMIT) -s -w
BUILD_FLAGS_CLI := -trimpath -ldflags "$(LDFLAGS_CLI)"

# Binary names
EDGELET_BINARY := build/edgelet
CLI_BINARY := $(EDGELET_BINARY)
DAEMON_BINARY := $(EDGELET_BINARY)

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: build-edgelet-local ## Build edgelet for host OS (linux thin or desktop monolithic)

build-edgelet: build-edgelet-local ## (alias) Build edgelet for host OS

build-edgelet-linux: deps ## Unified linux thin wrapper with embedded zstd tar (ARCH=amd64 default)
	@ARCH=$(or $(ARCH),amd64) STATIC_BUILD=$(STATIC_BUILD) ./scripts/build-edgelet

build-edgelet-local: ## Dev convenience binary at build/edgelet (host GOOS)
	@mkdir -p build
	@case "$$(uname -s)" in \
		Linux) \
			$(MAKE) build-edgelet-linux ARCH=$$(go env GOARCH); \
			cp build/edgelet-linux-$$(go env GOARCH) $(EDGELET_BINARY); \
			;; \
		Darwin) \
			CGO_ENABLED=0 GOOS=darwin GOARCH=$$(go env GOARCH) go build $(BUILD_FLAGS_EDGELET) -o $(EDGELET_BINARY) ./cmd/edgelet; \
			;; \
		MINGW*|MSYS*) \
			CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS_EDGELET) -o build/edgelet.exe ./cmd/edgelet; \
			;; \
		*) echo "Unsupported host OS for build-edgelet-local"; exit 1 ;; \
	esac
	@echo "Built: $(EDGELET_BINARY)"

build-cli: build-edgelet-local ## Build edgelet for CLI doc generation

cli-docs: build-cli ## Generate CLI markdown docs into docs/cli/generated
	@mkdir -p docs/cli/generated
	@$(CLI_BINARY) documentation generate md --output docs/cli/generated
	@find ./docs/cli/generated -type f | xargs sed -i '' 's/.*Auto generated.*//g'
	@find ./docs/cli/generated -type f | xargs sed -E -i '' 's/(command within \(default).*/\1 "default")/g'
	@echo "Generated docs/cli/generated/"

cli-docs-check: cli-docs ## Fail if docs/cli/ differs from committed generated output
	@git diff --exit-code docs/cli/ || (echo "ERROR: docs/cli drift — run 'make cli-docs' and commit" && exit 1)
	@echo "docs/cli/ is up to date"

cli-help-check: ## Fail if CLI help regression tests fail
	@go test ./internal/cli/cmd/ -run '^TestHelp_' -count=1
	@echo "CLI help regression tests passed"

cli-completion: build-cli ## Regenerate bash completion for packaging
	@mkdir -p packaging/edgelet/etc/bash_completion.d
	@$(CLI_BINARY) completion bash > packaging/edgelet/etc/bash_completion.d/edgelet
	@echo "Updated packaging/edgelet/etc/bash_completion.d/edgelet"

build-daemon: build-edgelet-local ## (alias) Build edgelet for host OS

build-daemon-embedded: build-edgelet-linux ## (alias) Build linux thin with embedded containerd bundle

deps: ## Download, build fat runtime, package embedded zstd bundle (linux; run before build-edgelet-linux)
	@echo "Building embedded data bundle for ARCH=$(or $(ARCH),amd64)..."
	@chmod +x scripts/clean scripts/download scripts/build-embedded scripts/package-data scripts/build-edgelet scripts/binary_size_check.sh scripts/ci 2>/dev/null || true
	@ARCH=$(or $(ARCH),amd64) ./scripts/download
	@ARCH=$(or $(ARCH),amd64) ./scripts/build-embedded
	@ARCH=$(or $(ARCH),amd64) ./scripts/build-edgelet fat
	@ARCH=$(or $(ARCH),amd64) ./scripts/package-data

# ── Linux release matrix (RFC R20; no musl-suffixed artifacts — RFC R9) ────────
# Optional ops-only static linking: STATIC_BUILD=true make build-linux-amd64

build-linux-amd64: ## Build unified linux edgelet for linux/amd64
	@$(MAKE) deps ARCH=amd64
	@ARCH=amd64 STATIC_BUILD=$(STATIC_BUILD) ./scripts/build-edgelet

build-linux-arm64: ## Build unified linux edgelet for linux/arm64
	@$(MAKE) deps ARCH=arm64
	@ARCH=arm64 STATIC_BUILD=$(STATIC_BUILD) ./scripts/build-edgelet

build-linux-arm: ## Build unified linux edgelet for linux/arm (armhf)
	@$(MAKE) deps ARCH=arm
	@ARCH=arm STATIC_BUILD=$(STATIC_BUILD) ./scripts/build-edgelet

build-linux-riscv64: ## Build unified linux edgelet for linux/riscv64
	@$(MAKE) deps ARCH=riscv64
	@ARCH=riscv64 STATIC_BUILD=$(STATIC_BUILD) ./scripts/build-edgelet

build-all-archs: build-linux-amd64 build-linux-arm64 build-linux-arm build-linux-riscv64 ## Build all linux targets (one binary per arch)

build-desktop-darwin: ## Build monolithic edgelet for darwin amd64+arm64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(BUILD_FLAGS_EDGELET) -o build/edgelet-darwin-amd64 ./cmd/edgelet
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(BUILD_FLAGS_EDGELET) -o build/edgelet-darwin-arm64 ./cmd/edgelet

build-desktop-windows: ## Build monolithic edgelet for windows/amd64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(BUILD_FLAGS_EDGELET) -o build/edgelet-windows-amd64.exe ./cmd/edgelet

release-tarballs: build-all-archs ## Package dist/edgelet-linux-<arch>.tar.gz and SHA256SUMS
	@chmod +x scripts/release-tarballs.sh
	@./scripts/release-tarballs.sh "$(VERSION)"

ci-docker: ## Run linux CI gate inside Docker (macOS-friendly)
	@docker build -f build/Dockerfile.embedded -t edgelet-embed-ci .
	@docker run --rm -v "$(CURDIR)":/src -w /src edgelet-embed-ci ./scripts/ci

test: ## Run unit tests (excludes build/src and build/gopath)
	@echo "Running tests..."
	@go test -v -tags '!linux' ./cmd/... ./internal/... ./pkg/... ./test/...
	@if [ "$$(uname -s)" = Linux ]; then \
		go test -v -tags linux ./cmd/... ./internal/... ./pkg/... ./test/...; \
	fi

test-unit: ## Run unit tests only (skip integration tests)
	@echo "Running unit tests..."
	@go test -v -short -tags '!linux' ./cmd/... ./internal/... ./pkg/... ./test/...
	@if [ "$$(uname -s)" = Linux ]; then \
		go test -v -short -tags linux ./cmd/... ./internal/... ./pkg/... ./test/...; \
		CGO_ENABLED=1 go test -v -short -tags 'linux,cgo' ./cmd/... ./internal/... ./pkg/... ./test/...; \
	fi

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v ./test/integration/...

test-embedded: ## Run embedded-containerd integration tests in a Lima VM (macOS only)
	@echo "Running embedded containerd integration tests..."
	@./test/embedded/run-all.sh

test-embedded-ci: ## Run embedded tests in CI mode (deletes VM on failure; Lima on macOS)
	@./test/embedded/run-all.sh --ci --delete-vm

test-coverage: ## Run tests with coverage
	@echo "Running tests with coverage..."
	@go test -v -coverprofile=build/coverage.out ./...
	@go tool cover -html=build/coverage.out -o build/coverage.html
	@echo "Coverage report: build/coverage.html"

benchmark: ## Run benchmarks
	@echo "Running benchmarks..."
	@go test -bench=. -benchmem ./...

profile: ## Run performance profiling (requires running agent)
	@echo "Running performance profiling..."
	@./scripts/profile.sh

# Download golangci-lint using the official install script if the binary is absent
# or if its version does not match GOLANGCI_LINT_VERSION.
$(GOLANGCI_LINT):
	@echo "⬇️  Installing golangci-lint $(GOLANGCI_LINT_VERSION) → $(GOBIN)..."
	@curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
		| sh -s -- -b $(GOBIN) $(GOLANGCI_LINT_VERSION)
	@echo "✓ golangci-lint $(GOLANGCI_LINT_VERSION) installed"

.PHONY: install-lint
install-lint: $(GOLANGCI_LINT) ## Install golangci-lint (pinned to GOLANGCI_LINT_VERSION)
	@$(GOLANGCI_LINT) version

lint: $(GOLANGCI_LINT) ## Run linters (auto-installs golangci-lint if needed)
	@echo "Running golangci-lint $(GOLANGCI_LINT_VERSION)..."
	@$(GOLANGCI_LINT) run --config .golangci.yaml

lint-fix: $(GOLANGCI_LINT) ## Run linters and auto-fix issues where possible
	@echo "Running golangci-lint $(GOLANGCI_LINT_VERSION) with --fix..."
	@$(GOLANGCI_LINT) run --config .golangci.yaml --fix

fmt: ## Format code
	@echo "Formatting code..."
	@go fmt ./...

vet: ## Run go vet
	@echo "Running go vet..."
	@go vet ./...

clean: ## Clean build artifacts
	@echo "Cleaning build artifacts..."
	@chmod +x scripts/clean 2>/dev/null || true
	@./scripts/clean
	@echo "Clean complete"

docker-build: ## Build production Docker image
	@echo "Building production Docker image..."
	@docker build -t iofog-agent-go:latest -t iofog-agent-go:$(VERSION) -f Dockerfile .
	@echo "Docker image built: iofog-agent-go:latest, iofog-agent-go:$(VERSION)"

docker-build-dev: ## Build development Docker image
	@echo "Building development Docker image..."
	@docker build -t iofog-agent-go:dev -f Dockerfile.dev .
	@echo "Docker image built: iofog-agent-go:dev"

install: build ## Install edgelet binary to system
	@echo "Installing edgelet..."
	@sudo cp $(EDGELET_BINARY) /usr/local/bin/edgelet
	@sudo chmod +x /usr/local/bin/edgelet
	@echo "Installed /usr/local/bin/edgelet"

# Development environment variables
DEV_DIR := $(shell pwd)/dev
DEV_CONFIG_DIR := $(DEV_DIR)/etc/edgelet
DEV_VAR_LIB := $(DEV_DIR)/var/lib/edgelet
DEV_VAR_LOG := $(DEV_DIR)/var/log/edgelet
DEV_VAR_RUN := $(DEV_DIR)/var/run/edgelet
DEV_PID_FILE := $(DEV_VAR_RUN)/edgelet.pid
DEV_CERT_FILE := $(DEV_CONFIG_DIR)/cert.crt

install-dev: build-edgelet-local ## Install edgelet and setup local dev environment
	@echo "Setting up local development environment..."
	@echo ""
	@# Create directory structure
	@mkdir -p $(DEV_CONFIG_DIR)
	@mkdir -p $(DEV_VAR_LIB)
	@mkdir -p $(DEV_VAR_LOG)
	@mkdir -p $(DEV_VAR_RUN)
	@echo "✓ Created directory structure"
	@echo ""
	@# Install edgelet multicall binary to /usr/local/bin (already in PATH)
	@echo "Installing edgelet to /usr/local/bin..."
	@sudo cp $(EDGELET_BINARY) /usr/local/bin/edgelet
	@sudo chmod +x /usr/local/bin/edgelet
	@echo "✓ Installed /usr/local/bin/edgelet"
	@echo ""
	@# Generate dev config.yaml if it doesn't exist
	@if [ ! -f $(DEV_CONFIG_DIR)/config.yaml ]; then \
		echo "Creating dev config.yaml..."; \
		printf '%s\n' \
			'currentProfile: development' \
			'' \
			'profiles:' \
			'  development:' \
			'    controllerUrl: "http://localhost:54421/api/v3/"' \
			'    iofogUuid: ""' \
			'    secureMode: "off"' \
			'    devMode: "on"' \
			'    controllerCert: "$(DEV_CERT_FILE)"' \
			'    arch: "auto"' \
			'    networkInterface: "dynamic"' \
			'    containerEngine: "docker"' \
			'    dockerUrl: "unix:///var/run/docker.sock"' \
			'    diskConsumptionLimit: "10"' \
			"    diskDirectory: \"$(DEV_VAR_LIB)/\"" \
			'    memoryConsumptionLimit: "4096"' \
			'    processorConsumptionLimit: "80.0"' \
			'    logDiskConsumptionLimit: "10.0"' \
			"    logDiskDirectory: \"$(DEV_VAR_LOG)/\"" \
			'    logFileCount: "10"' \
			'    logLevel: "DEBUG"' \
			'    statusFrequency: "30"' \
			'    getChangesFreq: "60"' \
			'    scanDevicesFreq: "60"' \
			'    gps: "auto"' \
			'    gpsCoordinates: "0,0"' \
			'    gpsDevice: ""' \
			'    gpsScanFrequency: "60"' \
			'    isolatedDockerContainer: "off"' \
			'    edgeGuardFrequency: "0"' \
			'    dockerPruningFrequency: "0"' \
			'    availableDiskThreshold: "20"' \
			'    upgradeScanFrequency: "24"' \
			'    timeZone: ""' \
			'    namespace: "default"' \
			'    caCert: ""' \
			'    tlsCert: ""' \
			'    tlsKey: ""' \
			> $(DEV_CONFIG_DIR)/config.yaml; \
		echo "✓ Created $(DEV_CONFIG_DIR)/config.yaml"; \
	else \
		echo "✓ Config file already exists at $(DEV_CONFIG_DIR)/config.yaml"; \
	fi
	@cp packaging/edgelet/etc/edgelet/cert_new.crt $(DEV_CONFIG_DIR)/cert.crt
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  Local Development Environment Setup Complete!"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "📁 Directory Structure:"
	@echo "   Config:     $(DEV_CONFIG_DIR)/config.yaml"
	@echo "   Data:       $(DEV_VAR_LIB)/"
	@echo "   Logs:       $(DEV_VAR_LOG)/"
	@echo "   Runtime:    $(DEV_VAR_RUN)/"
	@echo ""
	@echo "✅ edgelet installed to /usr/local/bin/ (already in your PATH)"
	@echo ""
	@echo "🚀 To start the edgelet daemon:"
	@echo "   make start-dev"
	@echo ""
	@echo "   Or manually:"
	@echo "   export SNAP_COMMON=$(DEV_DIR)"
	@echo "   edgelet daemon"
	@echo ""
	@echo "📝 You can edit the config at: $(DEV_CONFIG_DIR)/config.yaml"
	@echo "📋 View logs at: $(DEV_VAR_LOG)/"
	@echo ""
	@echo "💡 To use CLI commands directly, export SNAP_COMMON in your shell:"
	@echo "   export SNAP_COMMON=$(DEV_DIR)"
	@echo ""
	@echo "   Or add to your ~/.zshrc for persistence:"
	@echo "   echo 'export SNAP_COMMON=$(DEV_DIR)' >> ~/.zshrc"
	@echo "   source ~/.zshrc"
	@echo ""
	@echo "   Then you can use CLI commands directly:"
	@echo "   edgelet system status"
	@echo "   edgelet system info"
	@echo ""

start-dev: install-dev ## Start the edgelet daemon in development mode
	@echo "Starting edgelet daemon in development mode..."
	@# Check if already running
	@if [ -f $(DEV_PID_FILE) ]; then \
		PID=$$(cat $(DEV_PID_FILE) 2>/dev/null); \
		if ps -p $$PID > /dev/null 2>&1; then \
			echo "⚠️  edgelet daemon is already running (PID: $$PID)"; \
			echo "   Use 'make stop-dev' to stop it first"; \
			exit 1; \
		else \
			echo "⚠️  Removing stale PID file..."; \
			rm -f $(DEV_PID_FILE); \
		fi; \
	fi
	@# Start daemon in background
	@export SNAP_COMMON=$(DEV_DIR); \
	( nohup edgelet daemon > $(DEV_VAR_LOG)/daemon-startup.log 2>&1 & echo $$! > $(DEV_PID_FILE) ); \
	sleep 2; \
	PID=$$(cat $(DEV_PID_FILE) 2>/dev/null); \
	if [ -n "$$PID" ] && ps -p $$PID > /dev/null 2>&1; then \
		echo "✓ edgelet daemon started successfully (PID: $$PID)"; \
		echo ""; \
		echo "📋 View logs: tail -f $(DEV_VAR_LOG)/*.log"; \
		echo "🛑 Stop daemon: make stop-dev"; \
		echo "📊 Check status: ps aux | grep 'edgelet daemon'"; \
		echo ""; \
		echo "💡 Export SNAP_COMMON to use CLI commands directly:"; \
		echo "   export SNAP_COMMON=$(DEV_DIR)"; \
		echo "   edgelet system status"; \
	else \
		echo "❌ Failed to start edgelet daemon"; \
		echo "   Check logs: $(DEV_VAR_LOG)/daemon-startup.log"; \
		rm -f $(DEV_PID_FILE); \
		exit 1; \
	fi

stop-dev: ## Stop the edgelet daemon in development mode
	@echo "Stopping edgelet daemon..."
	@if [ ! -f $(DEV_PID_FILE) ]; then \
		echo "⚠️  PID file not found. Trying to find process..."; \
		PID=$$(pgrep -f '[e]dgelet daemon' | head -1); \
		if [ -z "$$PID" ]; then \
			echo "✓ No running edgelet daemon found"; \
			exit 0; \
		fi; \
		echo "   Found process with PID: $$PID"; \
		kill $$PID 2>/dev/null || true; \
		echo "✓ Stopped edgelet daemon (PID: $$PID)"; \
		exit 0; \
	fi; \
	PID=$$(cat $(DEV_PID_FILE) 2>/dev/null); \
	if [ -z "$$PID" ]; then \
		echo "⚠️  PID file is empty"; \
		rm -f $(DEV_PID_FILE); \
		exit 0; \
	fi; \
	if ps -p $$PID > /dev/null 2>&1; then \
		echo "   Stopping process $$PID..."; \
		kill $$PID 2>/dev/null || kill -9 $$PID 2>/dev/null || true; \
		sleep 1; \
		if ps -p $$PID > /dev/null 2>&1; then \
			echo "⚠️  Process still running, force killing..."; \
			kill -9 $$PID 2>/dev/null || true; \
		fi; \
		echo "✓ Stopped edgelet daemon (PID: $$PID)"; \
	else \
		echo "⚠️  Process $$PID not found (may have already stopped)"; \
	fi; \
	rm -f $(DEV_PID_FILE); \
	echo "✓ Cleanup complete"

# Development environment setup
setup-dev-env: install-dev ## Setup development environment and export SNAP_COMMON in current shell
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo "  Development Environment Setup"
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "To use CLI commands directly, run this in your shell:"
	@echo "  export SNAP_COMMON=$(DEV_DIR)"
	@echo ""
	@echo "Or add to your ~/.zshrc for persistence:"
	@echo "  echo 'export SNAP_COMMON=$(DEV_DIR)' >> ~/.zshrc"
	@echo "  source ~/.zshrc"
	@echo ""
	@echo "After exporting, you can use CLI commands directly:"
	@echo "  edgelet system status"
	@echo "  edgelet system info"
	@echo "  edgelet version"
	@echo ""
	@echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
	@echo ""
	@echo "💡 Quick setup - run this command in your terminal:"
	@echo "  export SNAP_COMMON=$(DEV_DIR)"
	@echo ""
	@echo "Or use: eval \$$(make export-dev-env)"
	@echo ""

export-dev-env: ## Print export command to set SNAP_COMMON for dev mode (usage: eval $(make export-dev-env))
	@echo "export SNAP_COMMON=$(DEV_DIR)"

build-size: build ## Show binary sizes
	@echo "Binary sizes:"
	@ls -lh build/edgelet* 2>/dev/null | awk '{print $$5 "\t" $$9}' || true

security-audit: ## Run dependency security audit
	@echo "🔐 Running dependency vulnerability scan..."

	@if ! command -v nancy >/dev/null 2>&1; then \
		echo "⬇️  Installing Nancy..."; \
		go install github.com/sonatype-nexus-community/nancy@latest; \
	fi

	@go list -json -deps ./... | nancy sleuth

	@echo "🔍 Verifying module integrity..."
	@go mod download
	@go mod verify


security-code: ## Run static Go security analysis
	@echo "🔍 Running Go static security analysis..."

	@if ! command -v gosec >/dev/null 2>&1; then \
		echo "⬇️  Installing gosec..."; \
		go install github.com/securego/gosec/v2/cmd/gosec@latest; \
	fi

	@gosec ./...
# --- Desktop App targets ---

DESKTOP_DIR := ../agent-desktop

desktop-deps: ## Install frontend npm deps for desktop app
	cd $(DESKTOP_DIR)/frontend && npm install

desktop-dev: desktop-deps ## Run desktop app in development mode (hot-reload)
	cd $(DESKTOP_DIR) && wails dev

desktop-app-darwin: desktop-deps ## Build macOS .app bundle (Wails desktop app)
	cd $(DESKTOP_DIR) && wails build -platform darwin/amd64,darwin/arm64 -o build/darwin/iofog-agent-desktop

desktop-app-windows: desktop-deps ## Build Windows .exe installer (Wails desktop app)
	cd $(DESKTOP_DIR) && wails build -platform windows/amd64 -o build/windows/iofog-agent-desktop.exe
