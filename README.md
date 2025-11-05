# pkgctl - Package Control Utility

A modern, type-safe package manager for Linux supporting multiple package formats (AppImage, DEB, RPM, Tarball, Binary).

## Features

- **Multi-Format Support**: Install and manage AppImage, DEB, RPM, tarball, ZIP, and binary packages
- **Desktop Integration**: Automatic `.desktop` file creation and icon management
- **Wayland/Hyprland Support**: Native Wayland environment variable injection
- **Database Tracking**: SQLite-based metadata storage for installed packages
- **Type-Safe**: Written in Go with compile-time guarantees
- **Testable**: Comprehensive test coverage with afero filesystem abstraction

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/diogo/pkgctl
cd pkgctl

# Build
make build

# Install
make install
```

### Build Requirements

- Go 1.21+
- make
- golangci-lint (optional, for development)

## Usage

### Install a Package

```bash
# Install an AppImage
pkgctl install myapp.AppImage

# Install a DEB package
pkgctl install myapp.deb

# Install a tarball
pkgctl install myapp.tar.gz
```

### List Installed Packages

```bash
# List all packages
pkgctl list

# JSON output
pkgctl list --json
```

### Show Package Information

```bash
pkgctl info myapp
```

### Uninstall a Package

```bash
pkgctl uninstall myapp
```

### System Health Check

```bash
pkgctl doctor
```

## Configuration

pkgctl looks for configuration in `~/.config/pkgctl/config.toml`:

```toml
[paths]
data_dir = "~/.local/share/pkgctl"
db_file = "~/.local/share/pkgctl/installed.db"
log_file = "~/.local/share/pkgctl/pkgctl.log"

[desktop]
wayland_env_vars = true

[logging]
level = "info"
color = "auto"
```

## Architecture

```
pkgctl/
├── cmd/pkgctl/          # Entry point
├── internal/            # Private application code
│   ├── core/           # Domain models and interfaces
│   ├── config/         # Configuration management
│   ├── logging/        # Structured logging
│   ├── db/             # SQLite database layer
│   ├── desktop/        # .desktop file handling
│   ├── icons/          # Icon management
│   ├── cache/          # Desktop/icon cache updates
│   ├── fsops/          # Filesystem operations
│   └── cmd/            # Cobra CLI commands
└── testdata/           # Test fixtures
```

## Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Open coverage report
make coverage
```

### Linting

```bash
# Run linter
make lint

# Format code
make fmt

# Run go vet
make vet

# Run all validation
make validate
```

### Adding a New Package Format

1. Implement the `Installer` interface in `internal/core/interfaces.go`
2. Add detection logic
3. Implement extraction and installation
4. Add comprehensive tests

## Project Status

- **Phase 0-1**: Foundation ✅ (Current)
- **Phase 2**: Backend Implementation (Planned)
- **Phase 3**: Advanced Features (Planned)
- **Phase 4**: Public API (Planned)

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development guidelines.

## License

[MIT License](LICENSE)

## Acknowledgments

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [zerolog](https://github.com/rs/zerolog) - Structured logging
- [modernc.org/sqlite](https://modernc.org/sqlite) - Pure Go SQLite
- [afero](https://github.com/spf13/afero) - Filesystem abstraction
