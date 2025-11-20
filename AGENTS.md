# upkg Agent Guidelines

## Commands
- **Build**: `make build` -> `bin/upkg`
- **Lint**: `make lint` (Fix all errors)
- **Test**: `make test` (Runs all tests)
- **Single Test**: `go test -v -race -run TestName ./path/to/pkg`
- **Validate**: `make validate` (Run before PR: fmt, vet, lint, test)

## Code Style & Conventions
- **Format**: Strict `go fmt` (run `make fmt`).
- **Naming**: PascalCase (exported), camelCase (private). CLI cmds: `kebab-case`.
- **Imports**: Group: Stdlib -> 3rd Party -> Local (`upkg/internal/...`).
- **Logging**: Use `internal/logging`. NEVER use `fmt.Printf` for logs.
- **Errors**: Always wrap errors with context (e.g., `fmt.Errorf("action: %w", err)`).
- **Security**: Validate paths/input via `internal/security`.
- **Testing**: Co-locate `*_test.go`. Use `testdata/` for fixtures. No logic in tests.
- **Structure**: `cmd/` (entry), `internal/core` (logic), `internal/backends` (pkg handlers).
