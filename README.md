# upkg - Modern Package Manager for Linux

[![Go Version](https://img.shields.io/badge/Go-1.25.3-blue.svg)](https://golang.org/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

**upkg** is a modern, type-safe package manager for Linux written in Go. It provides a unified interface for installing and managing applications from multiple package formats with full desktop integration, Wayland/Hyprland support, and SQLite-based tracking.

## ✨ Features

- **🔧 Multi-Format Support**: Install and manage packages from:
  - **AppImage** - Portable applications with desktop integration
  - **DEB** - Debian/Ubuntu packages (via debtap)
  - **RPM** - Red Hat/Fedora/SUSE packages (via rpmextract.sh or debtap)
  - **Tarball** - `.tar.gz`, `.tar.bz2`, `.tar.xz`, `.tgz`
  - **ZIP** - Compressed archives
  - **Binary** - Standalone ELF executables

- **🖥️ Desktop Integration**:
  - Automatic `.desktop` file generation
  - Icon extraction and installation (PNG, SVG, ICO, XPM)
  - Desktop database updates
  - Icon cache updates
  - **Wayland/Hyprland Native Support** with environment variable injection

- **💾 Database Tracking**:
  - SQLite-based metadata storage
  - Installation history and version tracking
  - Package integrity checks
  - Searchable package database

- **🛡️ Security & Safety**:
  - Path validation and traversal attack prevention
  - File integrity verification
  - Rollback capability on failed installations
  - Comprehensive logging

- **🎨 User Experience**:
  - Beautiful CLI with progress bars and colored output
  - Multiple output formats (table, JSON)
  - Advanced filtering and sorting
  - Fuzzy search support
  - System health diagnostics (`doctor` command)

## 📦 Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/quantmind-br/upkg.git
cd upkg

# Build
make build

# Install to $GOBIN or $GOPATH/bin
make install
```

### Build Requirements

- **Go 1.21+** (tested with Go 1.25.3)
- **make**
- **Optional**: golangci-lint (for development)

### Quick Start

After installation, you can immediately start installing packages:

```bash
# Install an AppImage
upkg install myapp.AppImage

# Install a DEB package (requires debtap)
upkg install google-chrome.deb

# Install a tarball
upkg install myapp.tar.gz
```

## 🚀 Usage

### Install Packages

```bash
# Install from file
upkg install application.AppImage

# Install with custom name
upkg install app.AppImage --name "My Application"

# Skip desktop integration
upkg install app.AppImage --skip-desktop

# Force reinstallation
upkg install app.AppImage --force
```

**Supported formats:**
- `*.AppImage` - AppImage packages
- `*.deb` - DEB packages (requires debtap)
- `*.rpm` - RPM packages (requires rpmextract.sh or debtap)
- `*.tar.gz`, `*.tar.bz2`, `*.tar.xz`, `*.tgz` - Tarball archives
- `*.zip` - ZIP archives
- ELF binaries - Standalone executables

### List Installed Packages

```bash
# List all packages
upkg list

# JSON output
upkg list --json

# Filter by type
upkg list --type appimage

# Filter by name (partial match)
upkg list --name firefox

# Sort by date
upkg list --sort date

# Detailed view
upkg list --details
```

### Show Package Information

```bash
upkg info application-name
```

### Uninstall Packages

```bash
upkg uninstall application-name
```

### System Health Check

```bash
# Basic check
upkg doctor

# Verbose with integrity checks
upkg doctor --verbose
```

The `doctor` command checks:
- Required dependencies (tar, unsquashfs)
- Optional dependencies (debtap, rpmextract.sh, desktop utilities)
- Directory structure and permissions
- Database integrity
- Package file integrity
- Environment variables

## ⚙️ Configuration

upkg uses a configuration file located at `~/.config/upkg/config.toml`:

```toml
[paths]
# Data directory for upkg files
data_dir = "~/.local/share/upkg"

# SQLite database file
db_file = "~/.local/share/upkg/installed.db"

# Log file location
log_file = "~/.local/share/upkg/upkg.log"

[desktop]
# Enable Wayland environment variable injection
wayland_env_vars = true

# Custom environment variables to inject
custom_env_vars = []

[logging]
# Log level: debug, info, warn, error
level = "info"

# Color output: always, never, auto
color = "auto"
```

### Wayland/Hyprland Support

When `wayland_env_vars = true`, upkg automatically injects the following environment variables into desktop entries:

```
GDK_BACKEND=wayland,x11
QT_QPA_PLATFORM=wayland;xcb
MOZ_ENABLE_WAYLAND=1
ELECTRON_OZONE_PLATFORM_HINT=auto
```

This ensures better compatibility with Wayland-based desktop environments.

## 🏗️ Architecture

upkg is designed with a modular, interface-driven architecture:

```
upkg/
├── cmd/upkg/          # Entry point
├── internal/
│   ├── core/           # Domain models and interfaces
│   ├── backends/       # Package format handlers
│   │   ├── appimage/   # AppImage backend
│   │   ├── deb/        # DEB backend
│   │   ├── rpm/        # RPM backend
│   │   ├── tarball/    # Tarball/ZIP backend
│   │   ├── binary/     # ELF binary backend
│   │   └── backend.go  # Backend registry
│   ├── config/         # Configuration management
│   ├── db/             # SQLite database layer
│   ├── desktop/        # Desktop entry handling
│   ├── icons/          # Icon management
│   ├── cache/          # Cache updates
│   ├── logging/        # Structured logging
│   ├── cmd/            # CLI commands
│   └── helpers/        # Utility functions
└── testdata/           # Test fixtures
```

### Backend Registry

upkg uses a priority-ordered backend registry for package detection:

1. **DEB and RPM** - Specific format detection
2. **AppImage** - Must come before Binary (AppImages are also ELF)
3. **Binary** - Standalone ELF executables
4. **Tarball/ZIP** - Archive formats

Each backend implements the `Backend` interface:
```go
type Backend interface {
    Name() string
    Detect(ctx context.Context, packagePath string) (bool, error)
    Install(ctx context.Context, packagePath string, opts InstallOptions) (*InstallRecord, error)
    Uninstall(ctx context.Context, record *InstallRecord) error
}
```

### Installation Flow

1. Package detection via backend registry
2. Format-specific installation
3. Desktop file generation/updates
4. Icon extraction and installation
5. Database record creation
6. Cache updates (desktop database, icon cache)

## 🔧 Development

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# View coverage report
make coverage
```

### Code Quality

```bash
# Format code
make fmt

# Run go vet
make vet

# Run linter
make lint

# Run all validation checks
make validate

# Quick check (fmt + vet + lint)
make quick-check

# Tidy go modules
make tidy
```

### Adding a New Package Format

1. Create a new backend directory: `internal/backends/<format>/`
2. Implement the `Backend` interface
3. Register the backend in `internal/backends/backend.go` (NewRegistry function)
4. Add comprehensive tests with afero filesystem mocking
5. Update documentation

See existing backends (e.g., `internal/backends/appimage/`) for examples.

## 📋 Package Format Details

### AppImage
- Extracts SquashFS filesystem
- Parses embedded `.desktop` files and metadata
- Copies AppImage to `~/.local/bin/`
- Automatic icon discovery (`.DirIcon`, embedded icons)
- Full desktop integration

### DEB
- Converts to Arch package using **debtap**
- Installs via **pacman** (requires sudo)
- Fixes malformed dependencies from debtap conversion
- Supports official package name extraction via `dpkg-deb`
- Updates system desktop database and icon cache

### RPM
- Two installation methods:
  1. **rpmextract.sh** (preferred) - Extracts and places files manually
  2. **debtap** (Arch) - Converts to Arch package and installs via pacman
- Smart executable detection with scoring heuristic
- Support for `rpm -qp` metadata extraction

### Tarball/ZIP
- Extracts archives to `~/.local/share/upkg/apps/`
- Advanced executable detection with scoring:
  - Filename matching
  - Directory depth preference
  - File size analysis
  - Pattern-based bonuses/penalties
- Wrapper script generation in `~/.local/bin/`
- **Electron app support** with ASAR icon extraction
- Special handling for invalid build paths

### Binary
- Simple ELF binary installation
- Copies to `~/.local/bin/`
- Creates generic `.desktop` entry
- Desktop integration

## 🗄️ Database Schema

upkg uses SQLite for tracking installations:

```sql
CREATE TABLE installs (
    install_id TEXT PRIMARY KEY,
    package_type TEXT NOT NULL,
    name TEXT NOT NULL,
    version TEXT,
    install_date DATETIME DEFAULT CURRENT_TIMESTAMP,
    original_file TEXT NOT NULL,
    install_path TEXT NOT NULL,
    desktop_file TEXT,
    metadata TEXT
);

CREATE INDEX idx_installs_name ON installs(name);
CREATE INDEX idx_installs_type ON installs(package_type);
```

Metadata is stored as JSON and includes:
- Icon file paths
- Wrapper script location
- Wayland support status
- Extracted metadata (categories, comments, etc.)

## 🔍 Command Reference

### install
Install a package from file.

**Flags:**
- `--force, -f`: Force installation even if already installed
- `--skip-desktop`: Skip desktop integration
- `--name, -n`: Custom application name
- `--timeout`: Installation timeout in seconds (default: 600)

### list
List installed packages.

**Flags:**
- `--json`: Output in JSON format
- `--type`: Filter by package type
- `--name`: Filter by package name (partial match)
- `--sort`: Sort by name, type, date, or version
- `--details, -d`: Show detailed information

### info
Show detailed information about a specific package.

### uninstall
Uninstall a package by name.

### doctor
Check system dependencies and integrity.

**Flags:**
- `--verbose, -v`: Verbose output with package integrity checks

### version
Show version information.

### completion
Generate shell completion script.

## 🔐 Security Considerations

- All package paths are validated to prevent directory traversal
- File integrity is verified during installation
- Failed installations are rolled back automatically
- SQLite database uses WAL mode for reliability
- Logging captures all operations for audit trails

## 📦 Dependencies

### Required
- **tar** - Extract tarball packages
- **unsquashfs** - Extract AppImage packages

### Optional
- **debtap** - Install DEB packages on Arch
- **rpmextract.sh** - Install RPM packages
- **gtk4-update-icon-cache** - Update icon cache
- **update-desktop-database** - Update desktop database
- **desktop-file-validate** - Validate desktop files
- **npx** - Extract icons from Electron ASAR archives
- **bsdtar** - Read package metadata (usually pre-installed)

## 🤝 Contributing

Contributions are welcome! Please read our contributing guidelines and feel free to submit issues and pull requests.

### Development Setup

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Run tests: `make test`
5. Run linter: `make lint`
6. Submit a pull request

## 📄 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [Cobra](https://github.com/spf13/cobra) - CLI framework
- [Viper](https://github.com/spf13/viper) - Configuration management
- [zerolog](https://github.com/rs/zerolog) - Structured logging
- [modernc.org/sqlite](https://modernc.org/sqlite) - Pure Go SQLite
- [afero](https://github.com/spf13/afero) - Filesystem abstraction
- [progressbar](https://github.com/schollz/progressbar/v3) - CLI progress indicators
- [tablewriter](https://github.com/olekukonko/tablewriter) - Table generation

## 🗺️ Roadmap

- [ ] **Phase 2**: Complete backend implementations
- [ ] **Phase 3**: Advanced features (updates, auto-update)
- [ ] **Phase 4**: Public API and plugin system

---

**Author**: Diogo
