
Based on my comprehensive analysis of the `.` project, I can now provide complete API documentation. This is a CLI-based package manager for Linux, not a web service, so the "APIs" are command-line interfaces and internal system integrations.

# API Documentation

## APIs Served by This Project

### Endpoints

#### `upkg install [package]`

- **Method and Path**: `CLI install [package_path]`
- **Description**: Installs a package from a local file, automatically detecting package type and performing desktop integration
- **Request**:
  - **Arguments**: `[package_path]` (string, required) - Path to package file
  - **Flags**:
    - `--force`, `-f` (bool): Force installation even if already installed
    - `--skip-desktop` (bool): Skip desktop integration
    - `--name`, `-n` (string): Custom application name
    - `--timeout` (int, default: 600): Installation timeout in seconds
    - `--skip-wayland-env` (bool): Skip Wayland environment variable injection
    - `--skip-icon-fix` (bool): Skip Hyprland dock icon fix
- **Response**:
  - **Success**: Prints package details (Name, Type, Install ID, Path, Desktop file)
  - **Error**: Returns exit codes 1-8 with descriptive error messages
- **Authentication**: File system permissions required
- **Examples**:
  ```bash
  upkg install /path/to/app.AppImage
  upkg install ./package.deb --name "Custom Name"
  ```

#### `upkg uninstall [identifier]`

- **Method and Path**: `CLI uninstall [package_name_or_install_id]`
- **Description**: Removes installed packages with interactive selection when no argument provided
- **Request**:
  - **Arguments**: `[identifier]` (string, optional) - Package name or install ID
  - **Flags**:
    - `--timeout` (int, default: 600): Uninstallation timeout in seconds
- **Response**:
  - **Success**: Confirmation message with space freed
  - **Error**: Package not found or cleanup failures
- **Authentication**: File system permissions required
- **Examples**:
  ```bash
  upkg uninstall myapp
  upkg uninstall abc123-def456
  upkg uninstall  # Interactive mode
  ```

#### `upkg list`

- **Method and Path**: `CLI list`
- **Description**: Lists installed packages with filtering and sorting options
- **Request**:
  - **Flags**:
    - `--json` (bool): Output in JSON format
    - `--type` (string): Filter by package type (appimage, binary, tarball, deb, rpm)
    - `--name` (string): Filter by package name (partial match)
    - `--sort` (string, default: "name"): Sort by name, type, date, version
    - `--details`, `-d` (bool): Show detailed information
- **Response**: Formatted table or JSON with package information
- **Authentication**: Read access to database required
- **Examples**:
  ```bash
  upkg list
  upkg list --type appimage --sort date
  upkg list --name firefox --details
  upkg list --json
  ```

#### `upkg info [identifier]`

- **Method and Path**: `CLI info [package_name_or_install_id]`
- **Description**: Shows detailed information about an installed package
- **Request**:
  - **Arguments**: `[identifier]` (string, required) - Package name or install ID
- **Response**: Detailed package information including paths, metadata, and installation details
- **Authentication**: Read access to database required
- **Examples**:
  ```bash
  upkg info myapp
  upkg info abc123-def456
  ```

#### `upkg doctor`

- **Method and Path**: `CLI doctor`
- **Description**: Performs system diagnostics and integrity checks
- **Request**:
  - **Flags**:
    - `--verbose`, `-v` (bool): Verbose output with integrity checks
- **Response**: System health report with issues and warnings
- **Authentication**: System access for dependency checking
- **Examples**:
  ```bash
  upkg doctor
  upkg doctor --verbose
  ```

#### `upkg completion [shell]`

- **Method and Path**: `CLI completion [bash|zsh|fish|powershell]`
- **Description**: Generates shell completion scripts
- **Request**:
  - **Arguments**: `[shell]` (string, required) - Shell type
- **Response**: Shell completion script to stdout
- **Authentication**: None
- **Examples**:
  ```bash
  upkg completion bash > /etc/bash_completion.d/upkg
  upkg completion zsh > "${fpath[1]}/_upkg"
  ```

#### `upkg version`

- **Method and Path**: `CLI version`
- **Description**: Displays version information
- **Request**: No arguments or flags
- **Response**: Version string
- **Authentication**: None
- **Examples**:
  ```bash
  upkg version
  ```

### Authentication & Security

- **Authentication**: Based on file system permissions of executing user
- **Security Features**:
  - **Path Validation**: Comprehensive validation in `internal/security/validation.go` prevents path traversal attacks
  - **Input Sanitization**: Package names and versions validated against dangerous patterns
  - **Transaction Management**: Atomic operations with rollback capability via `internal/transaction/manager.go`
  - **Command Injection Prevention**: All external commands use `exec.CommandContext` with separate arguments
  - **File Type Detection**: Magic number detection prevents execution of malicious files

### Rate Limiting & Constraints

- **Rate Limiting**: Not applicable (local CLI tool)
- **Constraints**:
  - Installation timeout: 600 seconds (configurable)
  - Uninstallation timeout: 600 seconds (configurable)
  - Package name length: Maximum 255 characters
  - Version string length: Maximum 100 characters
  - File path length: Maximum 4096 characters

## External API Dependencies

### Services Consumed

#### Local Database (SQLite)

- **Service Name & Purpose**: SQLite database for package metadata storage
- **Base URL/Configuration**: `cfg.Paths.DBFile` (default: `~/.local/share/upkg/installed.db`)
- **Endpoints Used**: 
  - `Create`: Store installation records
  - `Get`: Retrieve by ID or name
  - `List`: List all installations
  - `Delete`: Remove installation records
- **Authentication Method**: File system access
- **Error Handling**: Connection errors logged, transaction rollback on failures
- **Retry/Circuit Breaker Configuration**: WAL mode with 5-second busy timeout

#### Hyprland IPC

- **Service Name & Purpose**: Hyprland window manager integration for dock icon fixes
- **Base URL/Configuration**: Local Unix socket via `hyprctl` command
- **Endpoints Used**:
  - `hyprctl clients -j`: Get window client information
  - `hyprctl version`: Check if Hyprland is running
- **Authentication Method**: Local IPC (no authentication)
- **Error Handling**: Graceful degradation if Hyprland not available
- **Retry/Circuit Breaker Configuration**: Polling with configurable timeout

#### System Package Manager (Pacman)

- **Service Name & Purpose**: Arch Linux package manager integration
- **Base URL/Configuration**: Local `pacman` binary
- **Endpoints Used**:
  - `pacman -Qi`: Query package information
  - `pacman -Ql`: List package files
- **Authentication Method**: sudo elevation for install/remove operations
- **Error Handling**: Command execution errors captured and returned
- **Retry/Circuit Breaker Configuration**: None (single attempt)

#### System Utilities

- **Service Name & Purpose**: Essential system tools for package extraction
- **Base URL/Configuration**: System PATH
- **Endpoints Used**:
  - `tar`: Extract tarball archives
  - `unsquashfs`: Extract AppImage files
  - `dpkg`: DEB package operations (via debtap)
  - `rpm`: RPM package operations (via rpmextract.sh)
  - `desktop-file-validate`: Validate desktop files
  - `update-desktop-database`: Update desktop database
  - `gtk4-update-icon-cache`: Update icon cache
- **Authentication Method**: File system permissions
- **Error Handling**: Command execution wrapped with error capture
- **Retry/Circuit Breaker Configuration**: None (single attempt with timeout)

### Integration Patterns

1. **Backend Abstraction Pattern**: Interface-based design in `internal/core/interfaces.go` allows multiple package type handlers
2. **Transaction Pattern**: `internal/transaction/manager.go` ensures atomic operations with rollback capability
3. **Provider Pattern**: System package integration via `internal/syspkg/provider.go` interface
4. **Registry Pattern**: Backend discovery and selection in `internal/backends/backend.go`
5. **Configuration Pattern**: Centralized configuration management with environment variable overrides
6. **Security-First Pattern**: Comprehensive input validation and sanitization throughout

## Available Documentation

| Path | Description | Quality Evaluation |
|------|-------------|-------------------|
| `README.md` | Comprehensive project overview and usage guide | **High** - Complete with examples and configuration |
| `internal/core/interfaces.go` | Core API contracts and data models | **High** - Well-defined interfaces |
| `internal/config/config.go` | Configuration structure and defaults | **High** - Complete with path expansion |
| `.ai/docs/api_analysis.md` | Existing API analysis document | **High** - Detailed technical analysis |
| `.serena/memories/architecture_overview.md` | Internal architecture documentation | **High** - System design insights |
| `go.mod` | Dependencies and Go version requirements | **High** - Complete dependency list |

**Documentation Quality Evaluation**: The project has excellent internal documentation with well-defined interfaces, comprehensive README, and existing API analysis. The code follows Go best practices with clear separation of concerns and extensive error handling. The CLI commands are self-documenting via cobra's help system.