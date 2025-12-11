# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

upkg is a modern, type-safe package manager for Linux written in Go. It provides a unified interface for installing and managing applications from multiple package formats (AppImage, DEB, RPM, Tarball, ZIP, Binary). Key features include a transaction manager for installation reliability, full desktop integration, Wayland/Hyprland support, and SQLite-based tracking.

**Tech Stack:**
- Go
- SQLite (modernc.org/sqlite - pure Go implementation)
- Cobra (CLI framework)
- Viper (configuration management)
- zerolog (structured logging)
- afero (filesystem abstraction for testing)

**Documentation:**
- `AGENTS.md`: Condensed development guidelines for all AI agents.
- `pkg-test/`: Directory containing sample packages for testing.

## Development Commands

```bash
# Build the binary to bin/upkg
make build

# Install the binary to $GOBIN or $GOPATH/bin
make install

# Run all tests with the race detector enabled
make test

# Generate a test coverage report (coverage.html)
make test-coverage

# Run the linter (golangci-lint)
make lint

# Format all Go code with gofmt
make fmt

# Run the full validation suite (fmt + vet + lint + test)
make validate

# Run a quick check without running tests (fmt + vet + lint)
make quick-check
```

**Run a single test:** `go test -v -race -run TestName ./path/to/pkg`

**After any code modification, it's highly recommended to run:** `make validate`

## Code Architecture

### Backend Registry Strategy Pattern

The core of `upkg` is a **priority-ordered backend registry**, which is an implementation of the Strategy Pattern. This system detects and handles various package formats.

**Backend Interface:** (`internal/backends/backend.go`)
```go
type Backend interface {
    Name() string
    Detect(ctx context.Context, packagePath string) (bool, error)
    Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error)
    Uninstall(ctx context.Context, record *core.InstallRecord) error
}
```

**Registration Order is CRITICAL:** The order of registration in `internal/backends/backend.go`'s `NewRegistry()` function is crucial for correct detection.
1.  **DEB & RPM**: Specific, well-defined formats are checked first.
2.  **AppImage**: Must be checked *before* Binary, as AppImages are also valid ELF executables.
3.  **Binary**: Generic ELF executable detection.
4.  **Tarball/ZIP**: Archive formats are checked last as they are the most generic.

### Key Architectural Components

- **Transaction Manager** (`internal/transaction`): Provides atomic operations for installs/uninstalls. It uses a LIFO (Last-In, First-Out) stack of rollback functions. If any step fails, `Rollback()` is called to execute cleanup functions in the reverse order they were added, ensuring the system remains in a clean state.
- **Database Layer** (`internal/db`): A robust SQLite persistence layer using `modernc.org/sqlite`. It tracks all installed packages, storing metadata as a JSON blob. It uses separate read/write connection pools for performance.
- **Heuristics Engine** (`internal/heuristics`): Analyzes and scores potential executables within archives (Tarball, ZIP) to intelligently find the main application binary.
- **Desktop Integration** (`internal/desktop`): Manages the creation and validation of `.desktop` files, including injecting environment variables for Wayland/Hyprland compatibility.
- **Security Layer** (`internal/security`): Provides critical validation functions to prevent directory traversal attacks and sanitize user inputs.
- **System Package Provider** (`internal/syspkg`): An abstraction layer for interacting with the native OS package manager (e.g., `pacman` on Arch Linux). Used by backends like DEB and RPM.
- **Hyprland Integration** (`internal/hyprland`): Provides specific integration for the Hyprland compositor to fix common dock icon issues after installation.

### Installation Strategies

- **DEB**: Uses `debtap` to convert the package to a native Arch package, then installs it via `pacman`. Includes logic to remap Debian dependency names to their Arch equivalents.
- **RPM**:
    1.  **Strategy A (Preferred)**: Uses `rpmextract.sh` to manually extract contents into a managed directory (`~/.local/share/upkg/apps/`).
    2.  **Strategy B (Fallback)**: Uses `debtap` for conversion and `pacman` for installation.
- **AppImage**: Extracts the icon and `.desktop` file from the AppImage and integrates it directly.
- **Tarball/Zip**: Extracts the archive, uses the Heuristics Engine to find the main executable, creates a wrapper script, and generates a `.desktop` file.

### Key Directories

- `internal/backends/`: Format-specific handlers (appimage, deb, rpm, tarball, binary).
- `internal/cmd/`: CLI command definitions using Cobra.
- `internal/core/`: Core domain models and interfaces.
- `internal/db/`: SQLite database layer.
- `internal/desktop/`: `.desktop` file generation and management.
- `internal/heuristics/`: Executable detection and scoring logic.
- `internal/security/`: Path validation and input sanitization.
- `internal/syspkg/`: System package manager abstraction.
- `internal/transaction/`: The atomic transaction manager.
- `internal/ui/`: CLI progress bars, prompts, and spinners.

## Code Style & Conventions

- **Imports**: Group imports in three blocks: 1. Standard library, 2. Third-party packages, 3. Local project packages (`upkg/internal/...`).
- **Logging**: Use the structured logger from `internal/logging`. **NEVER** use `fmt.Printf` or `log.Printf` for application logging.
- **Errors**: Always wrap errors with context to create a clear error chain. Example: `return fmt.Errorf("failed to install package: %w", err)`.
- **Context**: Pass `ctx context.Context` as the first parameter to any function that performs I/O or can be long-running.
- **Security**: All file paths and user-provided identifiers **must** be validated using the `internal/security` package.
- **Testing**: Use the `afero` library for filesystem mocking to create hermetic tests. Test files (`*_test.go`) should be located in the same package as the code they are testing.

## External Dependencies

**Core Tools (Required):**
- `tar`, `unsquashfs` (for AppImage), `bsdtar`, `dpkg-deb` (for DEB), `rpm` (for RPM).

**System Integration (Arch Linux Specific):**
- `debtap`: Required for DEB backend and as a fallback for RPM.
- `pacman`: Required for installing packages via `debtap`.
- `rpmextract.sh`: The preferred tool for the RPM backend.

**Desktop Integration (Optional but Recommended):**
- `gtk4-update-icon-cache` / `gtk-update-icon-cache`
- `update-desktop-database`
- `desktop-file-validate`