.PHONY: build build-cli build-daemon build-daemon-lite build-daemon-full build-daemon-embedded deps test lint lint-fix clean docker-build docker-build-dev install install-dev start-dev stop-dev setup-dev-env export-dev-env fmt vet help build-all-archs build-linux-amd64 build-linux-amd64-musl build-linux-arm64 build-linux-arm64-musl build-linux-arm build-linux-riscv64 release-tarballs build-desktop-darwin build-desktop-windows desktop-dev test-embedded test-embedded-ci

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

# Flavor: lite (CGO=0, docker|podman) or full (CGO=1, embedded containerd / iofog)
FLAVOR ?= full

# Build flags
# CLI is flavor-agnostic; daemon carries flavor metadata.
LDFLAGS_CLI := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT) -s -w
LDFLAGS_DAEMON := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT) \
	-X github.com/eclipse-iofog/agent/internal/buildmeta.Flavor=$(FLAVOR) -s -w
BUILD_FLAGS_CLI := -trimpath -ldflags "$(LDFLAGS_CLI)"
BUILD_FLAGS_DAEMON := -trimpath -ldflags "$(LDFLAGS_DAEMON)"

# Fixed flavor ldflags for multi-flavor release builds (do not depend on FLAVOR=)
LDFLAGS_LITE := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT) \
	-X github.com/eclipse-iofog/agent/internal/buildmeta.Flavor=lite -s -w
LDFLAGS_FULL := -X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -X main.gitCommit=$(GIT_COMMIT) \
	-X github.com/eclipse-iofog/agent/internal/buildmeta.Flavor=full -s -w

# Binary names
CLI_BINARY := build/iofog-agent
DAEMON_BINARY := build/iofog-agentd

# Default target
.DEFAULT_GOAL := help

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: build-cli build-daemon ## Build both binaries for FLAVOR (default: full)

build-cli: ## Build CLI binary (flavor-agnostic)
	@echo "Building iofog-agent CLI..."
	@mkdir -p build
	@CGO_ENABLED=0 go build $(BUILD_FLAGS_CLI) -o $(CLI_BINARY) ./cmd/iofog-agent
	@echo "Built: $(CLI_BINARY)"

build-daemon: build-daemon-$(FLAVOR) ## Build daemon for current FLAVOR (default full)

build-daemon-lite: ## Lite daemon: CGO=0, external docker/podman only
	@echo "Building iofog-agentd lite..."
	@mkdir -p build
	@$(MAKE) FLAVOR=lite _build-daemon-cgo0

build-daemon-full: ## Full daemon: CGO=1, embedded containerd (iofog engine)
	@echo "Building iofog-agentd full..."
	@mkdir -p build
	@$(MAKE) FLAVOR=full _build-daemon-cgo1

.PHONY: _build-daemon-cgo0 _build-daemon-cgo1
_build-daemon-cgo0:
	@CGO_ENABLED=0 go build $(BUILD_FLAGS_DAEMON) -o $(DAEMON_BINARY) ./cmd/iofog-agentd

_build-daemon-cgo1:
	@CGO_ENABLED=1 go build $(BUILD_FLAGS_DAEMON) -tags cgo -o $(DAEMON_BINARY) ./cmd/iofog-agentd

deps: ## Download all embedded binary dependencies (run before build-daemon-embedded)
	@echo "Downloading embedded dependencies for ARCH=$(ARCH)..."
	@./build/download-deps.sh --os=linux --arch=$(or $(ARCH),amd64)

build-daemon-embedded: build-daemon-full ## (alias) Build full daemon with embedded containerd

# ── static cross-compilation targets (CGO_ENABLED=1 + musl toolchains) ─────────
build-linux-amd64: ## Build lite+full for linux/amd64 (glibc)
	@echo "Building for linux/amd64 (lite + full)..."
	@$(MAKE) deps ARCH=amd64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-amd64-lite ./cmd/iofog-agent
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS_LITE)" -o build/iofog-agentd-linux-amd64-lite ./cmd/iofog-agentd
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-amd64-full ./cmd/iofog-agent
	@CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=gcc go build -trimpath -ldflags "$(LDFLAGS_FULL)" -tags cgo -o build/iofog-agentd-linux-amd64-full ./cmd/iofog-agentd

build-linux-amd64-musl: ## Build lite+full for linux/amd64-musl (static daemon for full)
	@echo "Building for linux/amd64-musl (lite + full)..."
	@$(MAKE) deps ARCH=amd64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-amd64-musl-lite ./cmd/iofog-agent
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS_LITE)" -o build/iofog-agentd-linux-amd64-musl-lite ./cmd/iofog-agentd
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-amd64-musl-full ./cmd/iofog-agent
	@CGO_ENABLED=1 GOOS=linux GOARCH=amd64 CC=x86_64-linux-musl-gcc \
		go build -trimpath -ldflags "$(LDFLAGS_FULL) -extldflags '-static'" -tags cgo \
		-o build/iofog-agentd-linux-amd64-musl-full ./cmd/iofog-agentd

build-linux-arm64: ## Build lite+full for linux/arm64 (glibc)
	@echo "Building for linux/arm64 (lite + full)..."
	@$(MAKE) deps ARCH=arm64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-arm64-lite ./cmd/iofog-agent
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS_LITE)" -o build/iofog-agentd-linux-arm64-lite ./cmd/iofog-agentd
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-arm64-full ./cmd/iofog-agent
	@CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-gnu-gcc go build -trimpath -ldflags "$(LDFLAGS_FULL)" -tags cgo -o build/iofog-agentd-linux-arm64-full ./cmd/iofog-agentd

build-linux-arm64-musl: ## Build lite+full for linux/arm64-musl
	@echo "Building for linux/arm64-musl (lite + full)..."
	@$(MAKE) deps ARCH=arm64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-arm64-musl-lite ./cmd/iofog-agent
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS_LITE)" -o build/iofog-agentd-linux-arm64-musl-lite ./cmd/iofog-agentd
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-arm64-musl-full ./cmd/iofog-agent
	@CGO_ENABLED=1 GOOS=linux GOARCH=arm64 CC=aarch64-linux-musl-gcc \
		go build -trimpath -ldflags "$(LDFLAGS_FULL) -extldflags '-static'" -tags cgo \
		-o build/iofog-agentd-linux-arm64-musl-full ./cmd/iofog-agentd

build-linux-arm: ## Build lite+full for linux/arm (armhf)
	@echo "Building for linux/arm (armhf) (lite + full)..."
	@$(MAKE) deps ARCH=arm
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-arm-lite ./cmd/iofog-agent
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags "$(LDFLAGS_LITE)" -o build/iofog-agentd-linux-arm-lite ./cmd/iofog-agentd
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-arm-full ./cmd/iofog-agent
	@CGO_ENABLED=1 GOOS=linux GOARCH=arm GOARM=7 CC=arm-linux-gnueabihf-gcc \
		go build -trimpath -ldflags "$(LDFLAGS_FULL)" -tags cgo -o build/iofog-agentd-linux-arm-full ./cmd/iofog-agentd

build-linux-riscv64: ## Build lite+full for linux/riscv64
	@echo "Building for linux/riscv64 (lite + full)..."
	@$(MAKE) deps ARCH=riscv64
	@mkdir -p build
	@CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-riscv64-lite ./cmd/iofog-agent
	@CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -trimpath -ldflags "$(LDFLAGS_LITE)" -o build/iofog-agentd-linux-riscv64-lite ./cmd/iofog-agentd
	@CGO_ENABLED=0 GOOS=linux GOARCH=riscv64 go build -trimpath -ldflags "$(LDFLAGS_CLI)" -o build/iofog-agent-linux-riscv64-full ./cmd/iofog-agent
	@CGO_ENABLED=1 GOOS=linux GOARCH=riscv64 CC=riscv64-linux-gnu-gcc \
		go build -trimpath -ldflags "$(LDFLAGS_FULL)" -tags cgo -o build/iofog-agentd-linux-riscv64-full ./cmd/iofog-agentd

build-all-archs: build-linux-amd64 build-linux-amd64-musl build-linux-arm64 build-linux-arm64-musl build-linux-arm build-linux-riscv64 ## Build all 6 targets (lite + full per arch)

release-tarballs: build-all-archs ## Package build/release/*.tar.gz and SHA256SUMS-lite / SHA256SUMS-full
	@chmod +x scripts/release-tarballs.sh
	@./scripts/release-tarballs.sh "$(VERSION)"

test: ## Run tests
	@echo "Running tests..."
	@go test -v ./...

test-unit: ## Run unit tests only (skip integration tests)
	@echo "Running unit tests..."
	@go test -v -short ./...

test-integration: ## Run integration tests
	@echo "Running integration tests..."
	@go test -v ./test/integration/...

test-embedded: ## Run embedded-containerd integration tests in a Lima VM (macOS only)
	@echo "Running embedded containerd integration tests..."
	@./test/embedded/run-all.sh

test-embedded-ci: ## Run embedded tests in CI mode (deletes VM on failure)
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
	@rm -rf build/
	@echo "Clean complete"

docker-build: ## Build production Docker image
	@echo "Building production Docker image..."
	@docker build -t iofog-agent-go:latest -t iofog-agent-go:$(VERSION) -f Dockerfile .
	@echo "Docker image built: iofog-agent-go:latest, iofog-agent-go:$(VERSION)"

docker-build-dev: ## Build development Docker image
	@echo "Building development Docker image..."
	@docker build -t iofog-agent-go:dev -f Dockerfile.dev .
	@echo "Docker image built: iofog-agent-go:dev"

install: build ## Install binaries to system
	@echo "Installing binaries..."
	@sudo cp $(CLI_BINARY) /usr/local/bin/
	@sudo cp $(DAEMON_BINARY) /usr/local/bin/
	@echo "Binaries installed to /usr/local/bin/"

# Development environment variables
DEV_DIR := $(shell pwd)/dev
DEV_CONFIG_DIR := $(DEV_DIR)/etc/iofog-agent
DEV_VAR_LIB := $(DEV_DIR)/var/lib/iofog-agent
DEV_VAR_LOG := $(DEV_DIR)/var/log/iofog-agent
DEV_VAR_RUN := $(DEV_DIR)/var/run/iofog-agent
DEV_PID_FILE := $(DEV_VAR_RUN)/iofog-agentd.pid
DEV_CERT_FILE := $(DEV_CONFIG_DIR)/cert.crt

install-dev: build-cli build-daemon-lite ## Install binaries and setup local dev environment
	@echo "Setting up local development environment..."
	@echo ""
	@# Create directory structure
	@mkdir -p $(DEV_CONFIG_DIR)
	@mkdir -p $(DEV_VAR_LIB)
	@mkdir -p $(DEV_VAR_LOG)
	@mkdir -p $(DEV_VAR_RUN)
	@echo "✓ Created directory structure"
	@echo ""
	@# Install binaries to /usr/local/bin (already in PATH)
	@echo "Installing binaries to /usr/local/bin..."
	@sudo cp $(CLI_BINARY) /usr/local/bin/iofog-agent
	@sudo cp $(DAEMON_BINARY) /usr/local/bin/iofog-agentd
	@sudo chmod +x /usr/local/bin/iofog-agent /usr/local/bin/iofog-agentd
	@echo "✓ Installed binaries to /usr/local/bin/"
	@echo ""
	@# Generate dev config.yaml if it doesn't exist
	@if [ ! -f $(DEV_CONFIG_DIR)/config.yaml ]; then \
		echo "Creating dev config.yaml..."; \
		printf '%s\n' \
			'currentProfile: development' \
			'' \
			'profiles:' \
			'  development:' \
			'    routerHost: ""' \
			'    routerPort: ""' \
			'    routerUuid: ""' \
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
			'    postDiagnosticsFreq: "10"' \
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
	@ cp packaging/iofog-agent/etc/iofog-agent/cert_new.crt $(DEV_CONFIG_DIR)/cert.crt
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
	@echo "✅ Binaries installed to /usr/local/bin/ (already in your PATH)"
	@echo ""
	@echo "🚀 To start the agent daemon:"
	@echo "   make start-dev"
	@echo ""
	@echo "   Or manually:"
	@echo "   export SNAP_COMMON=$(DEV_DIR)"
	@echo "   iofog-agentd"
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
	@echo "   iofog-agent status"
	@echo "   iofog-agent info"
	@echo ""

start-dev: install-dev ## Start the agent daemon in development mode
	@echo "Starting ioFog Agent daemon in development mode..."
	@# Check if already running
	@if [ -f $(DEV_PID_FILE) ]; then \
		PID=$$(cat $(DEV_PID_FILE) 2>/dev/null); \
		if ps -p $$PID > /dev/null 2>&1; then \
			echo "⚠️  Agent daemon is already running (PID: $$PID)"; \
			echo "   Use 'make stop-dev' to stop it first"; \
			exit 1; \
		else \
			echo "⚠️  Removing stale PID file..."; \
			rm -f $(DEV_PID_FILE); \
		fi; \
	fi
	@# Start daemon in background
	@export SNAP_COMMON=$(DEV_DIR); \
	( nohup iofog-agentd > $(DEV_VAR_LOG)/daemon-startup.log 2>&1 & echo $$! > $(DEV_PID_FILE) ); \
	sleep 2; \
	PID=$$(cat $(DEV_PID_FILE) 2>/dev/null); \
	if [ -n "$$PID" ] && ps -p $$PID > /dev/null 2>&1; then \
		echo "✓ Agent daemon started successfully (PID: $$PID)"; \
		echo ""; \
		echo "📋 View logs: tail -f $(DEV_VAR_LOG)/*.log"; \
		echo "🛑 Stop daemon: make stop-dev"; \
		echo "📊 Check status: ps aux | grep iofog-agentd"; \
		echo ""; \
		echo "💡 Export SNAP_COMMON to use CLI commands directly:"; \
		echo "   export SNAP_COMMON=$(DEV_DIR)"; \
		echo "   iofog-agent status"; \
	else \
		echo "❌ Failed to start agent daemon"; \
		echo "   Check logs: $(DEV_VAR_LOG)/daemon-startup.log"; \
		rm -f $(DEV_PID_FILE); \
		exit 1; \
	fi

stop-dev: ## Stop the agent daemon in development mode
	@echo "Stopping ioFog Agent daemon..."
	@if [ ! -f $(DEV_PID_FILE) ]; then \
		echo "⚠️  PID file not found. Trying to find process..."; \
		PID=$$(pgrep -f "iofog-agentd" | head -1); \
		if [ -z "$$PID" ]; then \
			echo "✓ No running agent daemon found"; \
			exit 0; \
		else \
			echo "   Found process with PID: $$PID"; \
			kill $$PID 2>/dev/null || true; \
			echo "✓ Stopped agent daemon (PID: $$PID)"; \
			exit 0; \
		fi; \
	fi
	@PID=$$(cat $(DEV_PID_FILE) 2>/dev/null); \
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
		echo "✓ Stopped agent daemon (PID: $$PID)"; \
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
	@echo "  iofog-agent status"
	@echo "  iofog-agent info"
	@echo "  iofog-agent version"
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
	@ls -lh build/iofog-agent* build/iofog-agentd* 2>/dev/null | awk '{print $$5 "\t" $$9}'
	@echo ""
	@echo "Total size:"
	@du -sk build/iofog-agent* | awk '{sum+=$$1} END {printf "%.1fM\n", sum/1024}'

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

build-desktop-darwin: desktop-deps ## Build macOS .app bundle (amd64 + arm64)
	cd $(DESKTOP_DIR) && wails build -platform darwin/amd64,darwin/arm64 -o build/darwin/iofog-agent-desktop

build-desktop-windows: desktop-deps ## Build Windows .exe installer (amd64)
	cd $(DESKTOP_DIR) && wails build -platform windows/amd64 -o build/windows/iofog-agent-desktop.exe
