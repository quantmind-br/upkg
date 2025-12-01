# upkg Architecture Overview

## Purpose
Modern, type-safe package manager for Linux supporting multiple package formats (AppImage, DEB, RPM, Tarball, ZIP, Binary) with desktop integration, Wayland/Hyprland support, and SQLite tracking.

## Core Architecture Pattern: Backend Registry

### Backend Interface (internal/backends/backend.go:21-33)
All package formats implement the `Backend` interface:
- `Name()` - Backend identifier
- `Detect(ctx, packagePath)` - Detects if backend can handle package
- `Install(ctx, packagePath, opts, tx)` - Installs package with transaction support
- `Uninstall(ctx, record)` - Removes package

### Registry (internal/backends/backend.go)
Priority-ordered backend registry with **CRITICAL ordering**:

1. **DEB/RPM** - Specific format detection first
2. **AppImage** - MUST come before Binary (AppImages are ELF too)
3. **Binary** - Generic ELF detection
4. **Tarball/ZIP** - Archive formats last

**Why order matters:** AppImages are ELF executables, so if Binary comes first, AppImages will be misdetected.

### Transaction Manager (internal/transaction/manager.go)
- Manages rollback operations as a LIFO stack
- `Add(name, fn)` registers a rollback function
- `Rollback()` executes all rollbacks in reverse order on failure
- `Commit()` clears the stack on success
- All backends receive `*transaction.Manager` in `Install()` for atomic operations

### Heuristics System (internal/heuristics/)
- `Scorer` interface for executable scoring
- `ScoreExecutable()` calculates scores for candidate executables
- `ChooseBest()` selects optimal executable from candidates
- Used by tarball/binary backends for main executable detection

### System Package Provider (internal/syspkg/)
- `Provider` interface abstracts system package managers
- `internal/syspkg/arch/pacman.go` - Arch Linux pacman implementation
- Used by DEB/RPM backends for system-level installation

### Installation Flow

1. **Detection**: `Registry.DetectBackend()` - Iterates backends until match
2. **Transaction**: Backend receives `transaction.Manager` for atomic operations
3. **Installation**: Backend-specific `Install()` method
4. **Desktop Integration**: `internal/desktop/` - Generate/update .desktop files
5. **Icon Management**: `internal/icons/` - Extract icons (PNG, SVG, ICO, XPM)
6. **Database Recording**: `internal/db/` - SQLite record creation
7. **Cache Updates**: `internal/cache/` - Update desktop database & icon cache
8. **Commit/Rollback**: Transaction committed on success, rolled back on failure

## Directory Structure

```
internal/
├── backends/          # Package format handlers
│   ├── appimage/      # AppImage backend
│   ├── binary/        # ELF binary backend
│   ├── deb/           # DEB package backend
│   ├── rpm/           # RPM package backend
│   ├── tarball/       # Tarball/ZIP backend
│   └── backend.go     # Registry & interface
├── cache/             # Desktop database & icon cache updates
├── cmd/               # CLI commands (install, list, uninstall, doctor)
├── config/            # TOML configuration management
├── core/              # Domain models & interfaces
├── db/                # SQLite database layer
├── desktop/           # .desktop file generation/modification
├── heuristics/        # Executable detection and scoring
├── icons/             # Icon extraction & installation
├── logging/           # Structured logging (zerolog)
├── security/          # Path validation, traversal prevention
├── syspkg/            # System package manager abstraction
│   └── arch/          # Arch Linux (pacman) implementation
├── transaction/       # Transaction manager for atomic operations
└── ui/                # CLI UI components
cmd/upkg/              # Entry point (main.go)
```

## Key Components

### Configuration (internal/config/)
- TOML-based: `~/.config/upkg/config.toml`
- Paths: data_dir, db_file, log_file
- Desktop: wayland_env_vars, custom_env_vars
- Logging: level, color

### Database (internal/db/)
- SQLite with WAL mode
- Table: `installs` (install_id, package_type, name, version, install_date, original_file, install_path, desktop_file, metadata)
- Metadata: JSON blob (icons, wrapper scripts, wayland support)
- Indexes: name, package_type

### Desktop Integration (internal/desktop/)
- Generates .desktop files in `~/.local/share/applications/`
- Wayland support: Injects env vars (GDK_BACKEND, QT_QPA_PLATFORM, MOZ_ENABLE_WAYLAND, ELECTRON_OZONE_PLATFORM_HINT)
- Icon references

### Security (internal/security/)
- Path validation
- Directory traversal prevention
- Rollback on installation failures

## Tech Stack
- Go 1.25.3
- SQLite (modernc.org/sqlite - pure Go)
- Cobra (CLI framework)
- Viper (configuration)
- zerolog (structured logging)
- afero (filesystem abstraction for testing)

## Entry Point
`cmd/upkg/main.go` - Loads config → Initializes logger → Executes root command
