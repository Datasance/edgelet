# Integration Tests

This directory contains integration tests for Edgelet.

## Running Integration Tests

### Prerequisites

- Docker daemon running
- PoT controller (optional, for controller tests)
- Valid configuration file at the expected location

### Run All Integration Tests

```bash
go test -v ./test/integration/...
```

### Run Specific Test

```bash
go test -v ./test/integration/... -run TestDockerConnection
```

### Skip Integration Tests

Integration tests are skipped when using the `-short` flag:

```bash
go test -short ./...
```

## Test Categories

### Docker Tests (`docker_test.go`)

Tests Docker integration:
- Docker connectivity
- Image pulling
- Container lifecycle (create, start, stop, remove)

### Controller Tests (`controller_test.go`)

Tests controller communication:
- Controller connection
- HTTP API communication

**Note**: These tests require a running PoT controller instance.

### End-to-End Tests (`e2e_test.go`)

Tests full Edgelet workflows:
- Daemon startup sequence
- Graceful shutdown
- Multi-module interaction
- Offline mode operation

## Test Environment

Integration tests may require:
- Docker daemon access
- Network connectivity
- Configuration files
- Test fixtures

Ensure your environment is properly configured before running integration tests.
