# upkg Agent Guidelines

## Project Overview
`upkg` is a Go-based package manager for Linux supporting multiple formats (DEB, RPM, AppImage, etc.). The core architecture uses a Backend Registry (Strategy Pattern) for format handling and a Transaction Manager for reliable, atomic installations.

## Commands
- **Build**: `make build` (output to `bin/upkg`)
- **Lint**: `make lint` (uses golangci-lint)
- **Test**: `make test` (runs all tests with race detector)
- **Single Test**: `go test -v -race -run TestName ./path/to/pkg`
- **Full Validation**: `make validate` (runs fmt, vet, lint, test)

## Architecture & Key Components
- **Backend Registry (`internal/backends`)**: Priority-ordered strategy pattern for handling package types. Order is critical.
- **Transaction Manager (`internal/transaction`)**: Ensures atomic operations with a LIFO rollback stack for safety.
- **Database Layer (`internal/db`)**: SQLite database tracks all installed packages and their metadata (JSON serialized).
- **CLI Layer (`internal/cmd`)**: Cobra-based command structure.
- **Heuristics Engine (`internal/heuristics`)**: Scores and selects the best executable from archives.

## Code Style & Conventions
- **Format**: Strict `go fmt` is enforced (`make fmt`).
- **Imports**: Grouped: 1. Stdlib, 2. 3rd Party, 3. Local (`upkg/internal/...`).
- **Logging**: Use the structured logger from `internal/logging`. **NEVER** use `fmt.Printf` for logs.
- **Errors**: Always wrap errors with context: `fmt.Errorf("action failed: %w", err)`.
- **Security**: All user input and file paths **MUST** be validated using the `internal/security` package to prevent traversal attacks.
- **Testing**: Use `afero` for filesystem mocking. Test files are co-located with source (`*_test.go`).