# Code Structure Analysis

## Architectural Overview
The codebase follows a modular CLI architecture with a clear separation between the command-line interface (CLI) layer, a generic backend abstraction for package management, and specialized package format implementations. It utilizes a **Strategy Pattern** for package detection and installation, wrapped in a **Transaction-like** management system to ensure atomic operations (install/rollback) on the host filesystem.

## Core Components
- **CLI Layer (`internal/cmd/`)**: Built using the Cobra library, it handles user input, command orchestration, and user feedback.
- **Backend Registry (`internal/backends/`)**: A central dispatcher that manages multiple package format handlers. It implements the logic for detecting the appropriate format for a given file.
- **Package Backends**: Specialized modules for handling different Linux package formats:
  - `appimage`: Handles AppImage extraction and integration.
  - `deb` / `rpm`: Handles system-level package installation via native providers.
  - `binary`: Manages standalone ELF executables.
  - `tarball`: Handles compressed archives.
- **Database (`internal/db/`)**: A SQLite-based persistence layer using a Read/Write pool strategy (WAL mode) to track installed packages and their metadata.
- **Transaction Manager (`internal/transaction/`)**: A stack-based rollback mechanism that tracks filesystem changes during installation to allow for clean reversals on failure.

## Service Definitions
- **`Registry` Service**: Dispatches installation tasks to the correct backend based on file signature detection.
- **`Manager` (Transaction)**: Manages a LIFO (Last-In-First-Out) stack of `RollbackFunc` operations.
- **`DB` Service**: Provides thread-safe access to the local state store, managing schema migrations and CRUD operations for installation records.
- **`Scanner`/`Scorer` (Heuristics)**: Analyzes directory structures to identify primary executables and rank them based on relevance (e.g., matching package names).
- **`Desktop` Service**: Handles parsing, modification, and generation of `.desktop` files for XDG integration.

## Interface Contracts
- **`Backend` Interface**: Defines the contract for package handlers.
  - `Name() string`: Returns the unique identifier for the backend.
  - `Detect(ctx, path) (bool, error)`: Determines if the backend can handle the file.
  - `Install(ctx, path, opts, tx) (*Record, error)`: Performs the installation and registers rollback steps.
  - `Uninstall(ctx, record) error`: Removes the package using stored metadata.
- **`Provider` Interface (`syspkg`)**: Abstraction for system package managers (e.g., Pacman, Apt).
  - `Install`, `Remove`, `IsInstalled`, `GetInfo`, `ListFiles`.
- **`CommandRunner`**: Abstraction for executing shell commands, allowing for mocking in tests.

## Design Patterns Identified
- **Strategy Pattern**: Backends (AppImage, Deb, etc.) are interchangeable strategies for the installation process.
- **Registry Pattern**: The `Registry` struct centralizes the discovery and selection of backends.
- **Command Pattern**: Encapsulated by the Cobra CLI framework for handling distinct application actions.
- **Transaction Pattern (Unit of Work)**: The `transaction.Manager` captures operations and their compensations to ensure atomicity.
- **Dependency Injection**: Used extensively (e.g., `NewWithDeps`) to inject loggers, filesystems (afero), and command runners.

## Component Relationships
1. **CLI → Registry**: The CLI command calls the Registry to identify the package format.
2. **Registry → Backend**: The Registry iterates through registered Backends until one claims the file.
3. **Backend → Transaction Manager**: During installation, the Backend registers cleanup tasks with the Transaction Manager.
4. **CLI → DB**: After a successful backend installation, the CLI records the result in the SQLite database.
5. **Backend → Helpers/Heuristics**: Backends use shared utilities for archive extraction, ELF detection, and executable discovery.

## Key Methods & Functions
- **`Registry.DetectBackend`**: Performs magic number/signature checks to identify package types.
- **`Backend.Install`**: The core logic for transforming a package file into an installed application (extraction, permission setting, desktop integration).
- **`transaction.Manager.Add`**: Records a compensation function to be called if the process fails.
- **`desktop.InjectWaylandEnvVars`**: Modifies `.desktop` entries to add environment variables for Wayland compatibility.
- **`heuristics.FindExecutables`**: Recursively scans directories to find ELF binaries while excluding libraries.