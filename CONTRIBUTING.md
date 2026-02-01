# Contributing to ioFog Agent (Go)

Thank you for your interest in contributing to the ioFog Agent Go implementation!

## Development Setup

1. Fork the repository
2. Clone your fork
3. Navigate to `agent-go/` directory
4. Install dependencies: `go mod download`
5. Make your changes
6. Run tests: `make test`
7. Run linters: `make lint`
8. Format code: `make fmt`
9. Submit a pull request

## Code Style

- Follow Go standard formatting (`go fmt`)
- Run `go vet` before committing
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions focused and small

## Testing

- Write tests for new functionality
- Run `make test` before committing
- Aim for good test coverage

## Pull Request Process

1. Update CHANGELOG.md with your changes
2. Ensure all tests pass
3. Ensure code is formatted and linted
4. Update documentation if needed
5. Submit PR with clear description

## Migration Context

This is a migration from Java to Go. When implementing features:

- Reference the original Java codebase for behavior
- Maintain feature parity with Java implementation
- Follow the migration plan in `migration/` directory
- Coordinate with other agents if working on interdependent features

## Questions?

Open an issue or contact the maintainers.
