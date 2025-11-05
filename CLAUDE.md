# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

pkgctl is a modern, type-safe package manager for Linux written in Go. It supports multiple package formats (AppImage, DEB, RPM, Tarball, ZIP, Binary) with desktop integration, Wayland/Hyprland support, and SQLite-based tracking.

## Essential Commands

### Build & Run
```bash
make build              # Build binary to bin/pkgctl
make run                # Build and run
make install            # Install to $GOBIN or $GOPATH/bin
```

### Testing
```bash
make test               # Run all tests with race detection
make test-coverage      # Generate coverage report (coverage.html)
make coverage           # Generate and display coverage statistics
go test -v ./...                  # Run all tests
go test -v ./internal/backends/appimage/  # Run tests for specific backend
go test -v -run TestFunctionName  # Run single test
```

### Code Quality
```bash
make validate           # Run fmt, vet, lint, and test
make quick-check        # Run fmt, vet, and lint (no tests)
make lint               # Run golangci-lint
make fmt                # Format code with gofmt
make vet                # Run go vet
make tidy               # Tidy go modules
```

### CLI Usage Examples
```bash
./bin/pkgctl install myapp.AppImage
./bin/pkgctl list --json
./bin/pkgctl info myapp
./bin/pkgctl uninstall myapp
./bin/pkgctl doctor
```

## Architecture

### Core Design Principles
- **Interface-Driven**: All major components (Installer, DatabaseStore, Logger, FileSystem) are defined as interfaces in `internal/core/interfaces.go`
- **Backend Pattern**: Package format handlers implement the Backend interface and are registered in a Registry (priority-ordered detection)
- **Filesystem Abstraction**: Uses afero for testable filesystem operations
- **Type Safety**: Strong typing with domain models in `internal/core/models.go`

### Key Architectural Patterns

#### Backend Registry (internal/backends/backend.go)
Backends are registered in **priority order** to handle detection correctly:
1. DEB and RPM (specific format detection)
2. AppImage (must come before Binary since AppImages are also ELF)
3. Binary (catches standalone ELF binaries)
4. Tarball/Zip (archive formats)

Each backend implements:
```go
type Backend interface {
    Name() string
    Detect(ctx context.Context, packagePath string) (bool, error)
    Install(ctx context.Context, packagePath string, opts core.InstallOptions) (*core.InstallRecord, error)
    Uninstall(ctx context.Context, record *core.InstallRecord) error
}
```

#### Install Record Flow
1. Backend detects and installs package
2. Returns `*core.InstallRecord` with metadata
3. Record stored in SQLite database (internal/db/)
4. Desktop integration created if applicable (internal/desktop/)
5. Icon management handled (internal/icons/)
6. Desktop/icon cache updated (internal/cache/)

#### Configuration Management
- Uses Viper for config loading from `~/.config/pkgctl/config.toml`
- Defaults are set in `internal/config/config.go`
- Path expansion for `~` handled automatically

### Directory Structure
```
internal/
├── core/           # Domain models (models.go) and interfaces (interfaces.go)
├── backends/       # Package format handlers (appimage, deb, rpm, tarball, binary)
│   └── backend.go  # Backend registry and detection logic
├── config/         # Configuration management (Viper-based)
├── db/             # SQLite database operations (InstallRecord CRUD)
├── desktop/        # .desktop file generation and management
├── icons/          # Icon extraction and installation
├── cache/          # Desktop/icon cache updates (update-desktop-database, gtk-update-icon-cache)
├── fsops/          # High-level filesystem operations (copy, chmod, etc.)
├── helpers/        # Utilities (archive extraction, ELF detection, exec)
├── security/       # Path validation and security checks
├── logging/        # Structured logging (zerolog wrapper)
├── ui/             # CLI UI components (progress bars, prompts, colors)
└── cmd/            # Cobra command implementations (install, list, info, uninstall, doctor)

cmd/pkgctl/         # Main entry point
```

## Development Workflow

### Adding a New Package Format
1. Create new backend in `internal/backends/<format>/`
2. Implement the Backend interface (Name, Detect, Install, Uninstall)
3. Register backend in `internal/backends/backend.go` NewRegistry() function in the correct priority order
4. Add comprehensive tests (use afero for filesystem mocking)
5. Update documentation

### Testing Guidelines
- Use `afero.NewMemMapFs()` for filesystem mocking in tests
- Test files follow `*_test.go` naming convention
- Use table-driven tests where applicable (see `internal/config/config_test.go`)
- Run tests with race detection enabled (`-race` flag is default in Makefile)
- Aim for high coverage (use `make coverage` to check)

### Common Test Patterns
```go
// Filesystem mocking
fs := afero.NewMemMapFs()
backend := &MyBackend{fs: fs}

// Create test files
afero.WriteFile(fs, "/test/file", []byte("content"), 0644)

// Test detection/installation
can, err := backend.Detect(ctx, "/test/file")
```

### Logging
Use structured logging with zerolog:
```go
logger.Info().
    Str("package", name).
    Str("version", version).
    Msg("installing package")
```

### Error Handling
- Use context-aware operations where possible
- Return descriptive errors with context
- Use exit codes defined in `internal/core/models.go` (ExitSuccess, ExitInstallFailed, etc.)

## Important Implementation Details

### Wayland Support
- Desktop entries can have Wayland environment variables injected
- Configured via `config.toml`: `[desktop] wayland_env_vars = true`
- Wrapper scripts created for executables to set env vars

### Desktop Integration Flow
1. Extract or identify executable from package
2. Extract icon files (PNG, SVG, etc.)
3. Generate `.desktop` file in `~/.local/share/applications/`
4. Install icons to `~/.local/share/icons/hicolor/<size>/apps/`
5. Update desktop/icon caches

### Security Considerations
- Path validation in `internal/security/validation.go`
- Prevent path traversal attacks
- Validate package file integrity

### Database Schema
SQLite database stores InstallRecord structs with fields:
- InstallID (unique identifier)
- PackageType (deb, rpm, appimage, tarball, binary)
- Name, Version
- InstallDate
- OriginalFile, InstallPath
- DesktopFile
- Metadata (icons, wrapper scripts, Wayland support)

## Module Information
- Module: `github.com/diogo/pkgctl`
- Go Version: 1.25.3
- Key Dependencies:
  - Cobra (CLI framework)
  - Viper (configuration)
  - zerolog (structured logging)
  - modernc.org/sqlite (pure Go SQLite)
  - afero (filesystem abstraction)
  - progressbar (CLI progress indicators)

## PRP (Product Requirement Prompt) Workflow
This project includes a PRPs/ directory for structured development planning. PRPs combine traditional PRD specifications with AI-critical context layers (codebase references, implementation strategies, validation gates) to enable high-quality agentic code generation. See `PRPs/README.md` for details.
