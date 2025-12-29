# Dependency Analysis

## Internal Dependencies

The application follows a modular layered architecture with clear separation of concerns, although some tight coupling exists between the command layer and the database/backend layers.

*   **`cmd/upkg` (Entrypoint):** The main entry point that initializes the global configuration, logger, and executes the CLI root command.
*   **`internal/cmd` (CLI Layer):** Implements Cobra-based commands. It acts as the orchestrator, depending on almost all other internal modules (`config`, `backends`, `db`, `transaction`, `ui`, `security`, `hyprland`).
*   **`internal/backends` (Strategy Layer):** Implements the logic for different package formats. It uses a Registry pattern to manage specific backend implementations:
    *   `backends/appimage`, `backends/deb`, `backends/rpm`, `backends/tarball`, `backends/binary`.
*   **`internal/core` (Domain Layer):** Defines shared interfaces (`InstallOptions`) and data models (`InstallRecord`, `Metadata`, `PackageType`). It is the most imported internal module as it provides the common language for backends and commands.
*   **`internal/db` (Data Layer):** Manages SQLite-based persistence. It defines its own `Install` structure and handles schema migrations.
*   **`internal/transaction` (Utility Layer):** Provides a rollback mechanism for file-system operations, used by backends and commands to ensure atomicity.
*   **`internal/helpers`, `internal/security`, `internal/paths`, `internal/ui`:** Low-level utility modules providing filesystem operations, input validation, path resolution, and terminal UI components.

## External Dependencies

The project leverages several high-quality Go libraries for CLI, system interaction, and data handling:

*   **`github.com/spf13/cobra` & `github.com/spf13/viper`:** Core framework for CLI command structure and configuration management.
*   **`github.com/rs/zerolog`:** Structured logging throughout the application.
*   **`modernc.org/sqlite`:** A CGO-free SQLite implementation, ensuring easy cross-compilation and deployment.
*   **`github.com/spf13/afero`:** Filesystem abstraction used for mocking and testing, though its adoption across all backends appears to be in progress.
*   **`github.com/fatih/color`, `github.com/schollz/progressbar/v3`, `github.com/manifoldco/promptui`:** Terminal UI enhancements for better user experience.
*   **`github.com/ulikunitz/xz`, `layeh.com/asar`:** Specific archive format support (XZ and Electron ASAR).
*   **`github.com/stretchr/testify`:** Standard testing framework for assertions and mocking.

## Dependency Graph

The dependency structure is largely a directed acyclic graph (DAG) flowing from the entry point down to utility modules:

```text
cmd/upkg (Main)
 └── internal/cmd (CLI Commands)
      ├── internal/config
      ├── internal/logging
      ├── internal/db
      ├── internal/backends (Registry)
      │    ├── internal/backends/appimage
      │    ├── internal/backends/deb
      │    ├── ... (other backends)
      │    └── internal/core (Models)
      ├── internal/transaction
      ├── internal/security
      └── internal/ui
```

## Dependency Injection

The project uses a mix of manual Dependency Injection and factory patterns:

*   **Manual Constructor Injection:** Sub-backends (like `deb`, `rpm`) are initialized via `NewWithDeps(cfg, log, fs, runner)`, allowing the injection of configuration, loggers, and filesystem abstractions.
*   **Registry Pattern:** The `backends.Registry` centralizes the creation and discovery of backends, injected with common dependencies.
*   **Interface-based Injection:** The `helpers.CommandRunner` and `afero.Fs` interfaces are used to allow mocking of system commands and filesystem operations during unit testing.

## Potential Issues

*   **Tight Coupling in CLI Commands:** `internal/cmd/install.go` contains significant business logic, including database initialization, transaction management, and post-install "fixes" (like `fixDockIcon`). Moving this logic to a "Service" or "Manager" layer would improve testability and separate concerns.
*   **Partial Abstraction:** While `afero.Fs` is present in some constructors, several parts of the codebase still use the standard `os` package directly, which limits the effectiveness of the filesystem abstraction for testing.
*   **Duplication of Models:** There is slight duplication/translation between `internal/core.InstallRecord` and `internal/db.Install`. While this separates domain models from persistence models, the translation logic in the command layer adds boilerplate.
*   **Circular Dependency Risk:** The current structure avoids circular dependencies, but the high degree of cross-imports in `internal/cmd` makes it the most fragile part of the graph. Any attempt to move logic from `cmd` to a new sub-package must be careful not to import `cmd` back.