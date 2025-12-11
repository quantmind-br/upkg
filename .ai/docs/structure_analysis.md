
# Code Structure Analysis
## Architectural Overview
The `upkg` project is a modern, type-safe package manager for Linux written in Go, implementing a **Backend Registry Strategy Pattern** with transaction management. The architecture follows clean separation of concerns with distinct layers: CLI presentation (`internal/cmd`), core domain logic (`internal/core`), strategy implementations (`internal/backends`), and cross-cutting services (`internal/config`, `internal/logging`, `internal/db`). The system supports multiple package formats (AppImage, DEB, RPM, Tarball, ZIP, Binary) with desktop integration, Wayland/Hyprland support, and SQLite-based tracking.

## Core Components
| Component | Directory | Purpose and Responsibility |
| :--- | :--- | :--- |
| **Backend Registry** | `/internal/backends` | Central strategy pattern implementation managing package format handlers with priority-based detection |
| **Transaction Manager** | `/internal/transaction` | Provides atomic operations with LIFO rollback stack for installation reliability |
| **Core Domain Models** | `/internal/core` | Defines fundamental data structures (`InstallRecord`, `DesktopEntry`, `PackageType`) and installation options |
| **CLI Command Layer** | `/internal/cmd` | Implements user-facing commands (install, uninstall, list, doctor, info, version, completion) |
| **Database Layer** | `/internal/db` | SQLite-based persistence with separate read/write pools for package metadata tracking |
| **Desktop Integration** | `/internal/desktop` | Handles .desktop file generation, Wayland environment injection, and desktop database updates |
| **Security Layer** | `/internal/security` | Implements path validation and directory traversal attack prevention |
| **Heuristics Engine** | `/internal/heuristics` | Analyzes and scores executables for optimal main executable detection in archives |
| **System Package Provider** | `/internal/syspkg` | Abstracts native OS package manager interactions (pacman for Arch Linux) |
| **Configuration Service** | `/internal/config` | TOML-based configuration management with environment variable overrides |
| **Logging Service** | `/internal/logging` | Structured logging with zerolog for operational visibility |

## Service Definitions
1. **Backend Registry Service** (`internal/backends/backend.go`): Manages priority-ordered backend detection and selection, with critical ordering (DEB/RPM → AppImage → Binary → Tarball/ZIP) to prevent misidentification
2. **Transaction Management Service** (`internal/transaction/manager.go`): Provides atomic operations through rollback function registration and LIFO execution pattern
3. **Database Service** (`internal/db/db.go`): SQLite persistence with WAL mode, separate connection pools, and JSON metadata storage
4. **Desktop Integration Service** (`internal/desktop/desktop.go`): Generates and validates .desktop files with Wayland environment variable injection
5. **Security Service** (`internal/security/validation.go`): Validates filesystem paths and prevents directory traversal attacks
6. **Heuristics Service** (`internal/heuristics/scorer.go`): Scores and selects optimal executables from package archives
7. **Configuration Service** (`internal/config/config.go`): Loads TOML configuration with path expansion and environment variable support
8. **Logging Service** (`internal/logging/logger.go`): Provides structured logging with configurable levels and output formatting

## Interface Contracts
1. **Backend Interface** (`internal/backends/backend.go`): Core contract for package format handlers
   - `Name() string` - Backend identifier
   - `Detect(ctx context.Context, packagePath string) (bool, error)` - Package type detection
   - `Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error)` - Package installation
   - `Uninstall(ctx context.Context, record *core.InstallRecord) error` - Package removal

2. **SystemPackageProvider Interface** (`internal/syspkg/provider.go`): Abstracts native package manager interactions
   - System package installation and dependency management
   - Platform-specific implementations (Arch Linux pacman)

3. **Scorer Interface** (`internal/heuristics/scorer.go`): Defines executable scoring logic
   - `ScoreExecutable(path string) int` - Calculates executable suitability scores
   - `ChooseBest(candidates []string) string` - Selects optimal executable

## Design Patterns Identified
1. **Strategy Pattern**: Backend registry with pluggable package format handlers
2. **Transaction Pattern**: Atomic operations with rollback capability using LIFO stack
3. **Registry Pattern**: Priority-ordered backend registration and selection
4. **Factory Pattern**: Backend instantiation through registry
5. **Command Pattern**: CLI command encapsulation with Cobra framework
6. **Repository Pattern**: Database abstraction with separate read/write concerns
7. **Template Method**: Common installation workflow across backends with format-specific implementations
8. **Observer Pattern**: Desktop database and icon cache updates after installation

## Component Relationships
- **CLI → Core**: Command handlers instantiate transaction managers and call backend services
- **Core → Backends**: Registry selects appropriate backend based on package detection
- **Backends → Services**: Each backend utilizes desktop, security, heuristics, and system package services
- **Transaction → All**: Transaction manager coordinates rollback across all backend operations
- **Database → Core**: Persists installation records and metadata for all package types
- **Configuration → Global**: Provides settings to all components through dependency injection
- **Logging → Cross-cutting**: Provides structured logging throughout the application

## Key Methods & Functions
| Component | Key Methods | Responsibility |
| :--- | :--- | :--- |
| **Backend Registry** | `DetectBackend()`, `GetBackend()`, `ListBackends()` | Package type detection and backend selection |
| **Transaction Manager** | `Add()`, `Rollback()`, `Commit()` | Atomic operation management and rollback |
| **Database** | `Create()`, `Get()`, `List()`, `Delete()` | Package metadata persistence and retrieval |
| **Desktop Integration** | `Parse()`, `Write()`, `InjectWaylandEnvVars()` | .desktop file handling and Wayland support |
| **Security** | `ValidatePath()` | Path sanitization and traversal prevention |
| **Heuristics** | `ScoreExecutable()`, `ChooseBest()` | Executable detection and scoring |
| **Configuration** | `Load()`, `expandPath()` | Settings management and path resolution |

## Available Documentation
| Document Path | Evaluation of Documentation Quality |
| :--- | :--- |
| `/.serena/memories/architecture_overview.md` | **Excellent**: Comprehensive architectural overview with detailed component descriptions and installation flow |
| `/.serena/memories/critical_patterns.md` | **Excellent**: Critical implementation patterns and gotchas with specific code examples and ordering requirements |
| `/.ai/docs/structure_analysis.md` | **Good**: High-level structural analysis with component relationships and design patterns |
| `README.md` | **Good**: User-facing documentation with features, installation, and usage examples |
| `/.serena/memories/code_style_conventions.md` | **Medium**: Code consistency and organization guidelines |
| `AGENTS.md`, `CLAUDE.md` | **Low**: Development tooling configuration, not structural documentation |

The documentation quality is high, with the `.serena/memories/` directory containing particularly valuable architectural insights and critical implementation patterns that are essential for understanding the system's design decisions and constraints.