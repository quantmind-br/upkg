# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

upkg is a modern, type-safe package manager for Linux written in Go. It provides a unified interface for installing and managing applications from multiple package formats (AppImage, DEB, RPM, Tarball, ZIP, Binary) with full desktop integration, Wayland/Hyprland support, and SQLite-based tracking.

**Tech Stack:**
- Go 1.25.3
- SQLite (modernc.org/sqlite - pure Go implementation)
- Cobra (CLI framework)
- Viper (configuration management)
- zerolog (structured logging)
- afero (filesystem abstraction for testing)

**Documentation:**
- `AGENTS.md`: Development guidelines and agent protocols.
- `pkg-test/`: Directory containing sample packages for testing.

## Development Commands

```bash
make build              # Build binary to bin/upkg
make install            # Install to $GOBIN or $GOPATH/bin
make test               # Run all tests with race detector
make test-coverage      # Generate coverage report (coverage.html)
make lint               # Run golangci-lint
make fmt                # Format code with gofmt
make validate           # Run fmt + vet + lint + test (full validation)
make quick-check        # Run fmt + vet + lint (skip tests)
```

**Run single test:** `go test -v -race -run TestName ./path/to/pkg`

**After any code modification, run:** `make validate`

## Code Architecture

### Backend Registry Pattern

upkg uses a **priority-ordered backend registry** for package format detection and handling. This is the core architectural pattern.

**Backend Interface:** (internal/backends/backend.go)
```go
type Backend interface {
    Name() string
    Detect(ctx context.Context, packagePath string) (bool, error)
    Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error)
    Uninstall(ctx context.Context, record *core.InstallRecord) error
}
```

**Registration Order (CRITICAL):** The order in `NewRegistry()` matters:
1. DEB and RPM - Specific format detection first
2. AppImage - MUST come before Binary (AppImages are ELF executables too)
3. Binary - Generic ELF detection
4. Tarball/ZIP - Archive formats last

### Key Architectural Components

**Transaction Manager** (internal/transaction/manager.go):
- Manages rollback operations as a LIFO stack
- `Add(name, fn)` registers a rollback function
- `Rollback()` executes all rollbacks in reverse order on failure
- `Commit()` clears the stack on success

**Heuristics System** (internal/heuristics/):
- `Scorer` interface for executable scoring
- `ScoreExecutable()` calculates scores (filename, depth, size, patterns)
- `ChooseBest()` selects optimal executable for Tarball/Binary backends

**System Package Provider** (internal/syspkg/):
- Abstracts system package managers (pacman, apt, etc.)
- Used by DEB/RPM backends for system-level installation via `debtap` or similar tools

**Hyprland Integration** (internal/hyprland/):
- `Client` struct and methods (`GetClients`, `WaitForClient`)
- Enables precise window matching and management for installed apps

### Installation Strategies

- **DEB**: Uses `debtap` to convert to Arch package -> `pacman` install. Includes **dependency fixing logic** (remapping Debian names to Arch names).
- **RPM**:
    1. Strategy A (Preferred): `rpmextract.sh` for manual extraction and installation to `~/.local/share/upkg/apps/`.
    2. Strategy B (Fallback): `debtap` conversion -> `pacman` install.
- **AppImage**: Integrated directly, extracts icon/desktop file.
- **Tarball/Zip**: Extracts, scores executables, creates wrapper & desktop integration.

### Key Directories

- `internal/backends/` - Format-specific backends (appimage, deb, rpm, tarball, binary)
- `internal/transaction/` - Transaction manager
- `internal/heuristics/` - Executable detection/scoring
- `internal/syspkg/` - System package manager abstraction
- `internal/hyprland/` - Wayland/Hyprland window management
- `internal/desktop/` - .desktop file generation
- `internal/icons/` - Icon extraction
- `internal/ui/` - CLI progress bars and spinners
- `internal/db/` - SQLite database layer
- `internal/config/` - TOML-based config
- `internal/security/` - Path validation

## Code Style & Conventions

- **Imports**: Group as Stdlib -> 3rd Party -> Local (`upkg/internal/...`)
- **Logging**: Use `internal/logging`. NEVER use `fmt.Printf` for logs
- **Errors**: Always wrap with context: `fmt.Errorf("action: %w", err)`
- **Context**: Use `ctx context.Context` as first parameter for I/O operations
- **Security**: Validate paths/input via `internal/security`
- **Testing**: Use `afero` filesystem mocking; co-locate `*_test.go` with source

**Linting:** (from .golangci.yml)
- Max cyclomatic complexity: 15
- Security: G204 excluded (subprocess with variable)

## External Dependencies

**Core:**
- `tar`, `unsquashfs` (AppImage)
- `bsdtar` (Archive extraction/repacking)
- `dpkg-deb` (DEB metadata)
- `rpm` (RPM metadata)

**System Integration (Arch Linux):**
- `debtap` (Required for DEB, Fallback for RPM)
- `pacman` (Required for DEB/RPM via debtap)
- `rpmextract.sh` (Preferred for RPM)

**Optional:**
- `gtk4-update-icon-cache`, `update-desktop-database`
