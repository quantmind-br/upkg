# Repository Guidelines

## Project Structure & Module Organization
- `cmd/pkgctl` holds the CLI entrypoint; the compiled binary lands in `bin/pkgctl`.
- Core logic lives under `internal/`: use `internal/backends` for package-format handlers, `internal/core` for shared types and interfaces, and `internal/cmd` for Cobra command wiring.
- Supporting subsystems include `internal/db` (SQLite persistence), `internal/desktop` and `internal/icons` (desktop integration), plus `internal/security` for safety checks.
- Tests rely on fixtures in `testdata/` and real-world archives under `pkg-test/` for manual verification.

## Build, Test & Development Commands
- `make build` produces the binary with version info from `git describe`.
- `make run` builds then executes `pkgctl` locally.
- `make validate` runs `fmt`, `vet`, `golangci-lint`, and the race-enabled test suite; use before every PR.
- `make tidy` keeps module metadata clean; run after adding dependencies.
- Targeted work: `go test ./internal/backends/appimage/...` or `go test -run TestName ./internal/...`.

## Coding Style & Naming Conventions
- All Go code must be `gofmt`-clean; `make fmt` enforces formatting, tabs, and import ordering (`goimports` is enabled in `.golangci.yml`).
- Favor small, single-responsibility packages; exported symbols use PascalCase, private helpers stay lowerCamelCase, and filenames use snake_case (e.g., `install_options.go`).
- Keep context-aware functions accepting `context.Context` first, return `(value, error)` pairs, and wrap errors with `fmt.Errorf("context: %w", err)`.

## Testing Guidelines
- `make test` runs the full suite with `-race`; `make test-coverage` emits `coverage.out` and `coverage.html`.
- Add table-driven `*_test.go` files alongside the code they verify; name tests `TestPackage_Feature`.
- Use the afero in-memory filesystem for install flows, and stage larger archives under `testdata/` to avoid side effects.
- Strive to keep critical backends and security checks at or above existing coverage; update golden data when behavior changes intentionally.

## Commit & Pull Request Guidelines
- Follow Conventional Commits (`feat(backends): add rpm checksum verification`); keep subjects ≤72 chars and include a short body with rationale and test evidence.
- Reference issues with `Refs #123` or `Fixes #123` when applicable.
- Pull requests need: concise summary, validation steps (e.g., `make validate` output), and screenshots/log snippets for user-visible changes.

## Security & Configuration Tips
- Never install untrusted archives outside a disposable environment; rely on `internal/security` helpers for path validation.
- Document changes that affect `~/.config/pkgctl/config.toml`, and call out new permissions or environment flags so downstream agents can reproduce installs safely.
