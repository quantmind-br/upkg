# API Analysis

## Project Type
This project is a CLI application (`upkg`) and internal library written in Go. It is a package manager utility for Linux that supports various package formats.

## Endpoints Overview
No HTTP/REST/GraphQL/gRPC endpoints - this is a CLI tool.

## Authentication
Authentication is not applicable for this tool as it runs locally. However, some package installations may require elevated privileges depending on the install path and package type (e.g., system-wide vs. local install).

## Detailed Endpoints
N/A

## Programmatic API (Internal Library)
The project is structured such that its internal logic can be used programmatically within the Go ecosystem.

### Database API (`internal/db`)
The database module manages the persistent record of installed packages using SQLite.

- **`New(ctx context.Context, dbPath string) (*DB, error)`**: Initializes a new database connection with separate read/write pools.
- **`DB.Create(ctx context.Context, install *Install) error`**: Persists a new installation record.
- **`DB.Get(ctx context.Context, installID string) (*Install, error)`**: Retrieves a record by its unique ID.
- **`DB.List(ctx context.Context) ([]Install, error)`**: Returns all tracked installations.
- **`DB.Update(ctx context.Context, install *Install) error`**: Updates an existing installation record.
- **`DB.Delete(ctx context.Context, installID string) error`**: Removes a record from the database.

### Backend API (`internal/backends`)
Backends handle the specifics of different package formats (AppImage, DEB, RPM, Tarball, Binary).

- **`Registry`**: Manages available backends and facilitates package type detection.
    - `NewRegistry(cfg *config.Config, log *zerolog.Logger) *Registry`
    - `DetectBackend(ctx context.Context, packagePath string) (Backend, error)`
- **`Backend` Interface**:
    - `Name() string`: Returns the identifier for the backend.
    - `Detect(ctx, packagePath) (bool, error)`: Checks if the backend can handle the file.
    - `Install(ctx, packagePath, opts, tx) (*core.InstallRecord, error)`: Executes installation.
    - `Uninstall(ctx, record) error`: Executes uninstallation.

## CLI API
The application uses the Cobra library to expose the following commands:

### `upkg install [package]`
Installs a package from a local file path.
- **Arguments**: Path to the package file (`.AppImage`, `.deb`, `.rpm`, `.tar.gz`, etc.).
- **Flags**:
    - `-f, --force`: Force installation even if already installed.
    - `--skip-desktop`: Skip desktop entry and icon integration.
    - `-n, --name [string]`: Provide a custom application name.
    - `--timeout [int]`: Set installation timeout in seconds (default: 600).
    - `--skip-wayland-env`: Skip Wayland environment variable injection.
    - `--skip-icon-fix`: Skip the interactive dock icon fix for Hyprland.
    - `--overwrite`: Overwrite conflicting files (specific to DEB/RPM).

### `upkg uninstall [package-name...]`
Removes one or more installed packages.
- **Arguments**: One or more package names or Install IDs.
- **Flags**:
    - `-y, --yes`: Skip confirmation prompts (required for non-interactive shells).
    - `--dry-run`: Preview which files and records would be removed.
    - `--all`: Uninstall all packages tracked by upkg.
    - `--timeout [int]`: Set uninstallation timeout in seconds.

### `upkg list`
Lists all packages currently tracked in the local database.
- **Flags**:
    - `--json`: Output the list in JSON format for scripting.
    - `--type [string]`: Filter by package type (e.g., `appimage`, `deb`).
    - `--name [string]`: Filter by partial name match.
    - `--sort [string]`: Sort results by `name`, `type`, `date`, or `version`.
    - `-d, --details`: Show detailed installation information in table view.

### `upkg info [identifier]`
Shows detailed metadata and file paths for a specific package.
- **Arguments**: Package name or Install ID.

### `upkg doctor`
Performs system diagnostics to ensure dependencies and directories are correctly configured.
- **Flags**:
    - `-v, --verbose`: Enable detailed integrity checks of installed package files.
    - `--fix`: Automatically create missing directories or fix permissions where possible.

## Common Patterns
- **Transaction Management**: The tool uses a `transaction.Manager` to track file system changes and provide rollback capabilities if an installation fails before being committed to the database.
- **Desktop Integration**: Automatically creates `.desktop` files in `~/.local/share/applications` and installs icons to `~/.local/share/icons`.
- **Environment Injection**: For GUI applications, it often wraps the executable to inject necessary environment variables (e.g., `XDG_SESSION_TYPE=wayland`).