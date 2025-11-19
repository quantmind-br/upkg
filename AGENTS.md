# Agent Guidelines for upkg

## Commands
- **Validate**: `make validate` (runs fmt, vet, lint, test). Run before requesting review.
- **Test**: `make test` (runs all). **Single Test**: `go test -v -race -run TestName ./path/to/pkg`
- **Lint**: `make lint` (golangci-lint). Fix all linter errors.
- **Build**: `make build` creates `bin/upkg`.

## Code Style & Conventions
- **Formatting**: strict `go fmt`.
- **Naming**: PascalCase for exported, camelCase for internal. lowercase-hyphen for CLI cmds.
- **Imports**: Group stdlib, then 3rd party, then local (`upkg/internal/...`).
- **Logging**: MUST use `internal/logging`. No `fmt.Printf` for logs.
- **Security**: Use `internal/security` for path/input validation.
- **Errors**: Return wrapped errors with context. Handle all errors.
- **Tests**: Unit tests in `*_test.go` next to code. Use `testdata/` for fixtures.

## Structure
- `cmd/upkg`: CLI entrypoint only.
- `internal/core`: Business logic/orchestration.
- `internal/backends`: AppImage, DEB, RPM handlers.
