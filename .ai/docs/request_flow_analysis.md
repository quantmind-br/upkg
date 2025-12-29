# Request Flow Analysis

The `upkg` utility is a CLI-based package manager. As such, its "request flow" refers to the lifecycle of a command execution from the terminal to the underlying system changes and final output.

## API Endpoints

Since `upkg` is a CLI tool, its "endpoints" are the subcommands provided by the `cobra` library. All commands are rooted at `upkg` (the `RootCmd`).

| Command | Arguments | Description |
|---------|-----------|-------------|
| `install` | `[package]` | Detects and installs a package (AppImage, DEB, RPM, Tarball, Binary). |
| `uninstall` | `[package-id]` | Removes an installed package and its associated metadata. |
| `list` | None | Lists all packages currently managed by `upkg`. |
| `info` | `[package-id]` | Shows detailed metadata and files associated with a package. |
| `doctor` | None | Performs system checks and validates package integrity. |
| `version` | None | Displays the application version. |
| `completion`| `[shell]` | Generates shell completion scripts (bash, zsh, fish, powershell). |

## Request Processing Pipeline

When a user executes a command, the request flows through the following pipeline:

1.  **Entry Point (`cmd/upkg/main.go`)**: The application initializes the configuration and logger, then passes control to the `internal/cmd/root.go`.
2.  **Command Validation (`spf13/cobra`)**: Cobra validates arguments (e.g., `ExactArgs(1)`) and flags.
3.  **Sanitization & Security (`internal/security`)**: Paths and package names are validated and sanitized (e.g., `security.ValidatePath`, `security.SanitizeString`) before processing.
4.  **Database Initialization (`internal/db`)**: A connection to the local SQLite database (typically `~/.config/upkg/upkg.db`) is established to track the state.
5.  **Context & Timeout**: A `context.Context` is created, often with a timeout (default 300s), to ensure long-running operations like downloads or extractions can be cancelled.
6.  **Transaction Management (`internal/transaction`)**: A transaction manager is initialized to track file system changes and allow for rollback in case of failure.

## Routing Logic

Routing in `upkg` is handled by the `cobra` command hierarchy and a specialized **Backend Registry**:

1.  **Command Routing**: Cobra routes the execution to the `RunE` function of the specific subcommand (e.g., `NewInstallCmd`).
2.  **Backend Detection (`internal/backends/registry.go`)**: For commands like `install`, the application uses a priority-based registry to detect the package type:
    *   **Priority 1**: DEB and RPM (via magic bytes/headers).
    *   **Priority 2**: AppImage (via ELF headers and AppImage-specific signatures).
    *   **Priority 3**: Standalone ELF Binaries.
    *   **Priority 4**: Archives (Tarball/Zip).
3.  **Dynamic Dispatch**: Once detected, the request is routed to the specific `Backend` implementation (e.g., `appimage.Backend`) which satisfies the `Backend` interface (`Detect`, `Install`, `Uninstall`).

## Response Generation

Response generation is handled through the **UI Layer** (`internal/ui`):

1.  **Standard Output (Stdout)**: General information and success messages are printed using the `ui` helper (e.g., `ui.PrintSuccess`).
2.  **Structured Data**: For programmatic use, commands like `list` and `info` support a `--json` flag, which bypasses the table-based UI and encodes the internal models directly to JSON.
3.  **Progress Tracking (`internal/ui/progress.go`)**: For long-running operations (extracting, moving files), a progress bar is displayed to the user.
4.  **Colorization**: Output is color-coded using `fatih/color` to differentiate between package types, success messages (Green), warnings (Yellow), and errors (Red).

## Error Handling

Error handling is implemented at multiple levels of the flow:

1.  **Error Propagation**: Errors are propagated up the call stack using `fmt.Errorf("...: %w", err)`.
2.  **User Notifications**: Subcommands use `ui.PrintError` to display human-friendly error messages to `os.Stderr`.
3.  **Transaction Rollback**: If an operation fails during `install`, the `transaction.Manager` executes a rollback, attempting to delete partially created files or directories to prevent system pollution.
4.  **Exit Codes**: The `main` function captures the error from `RootCmd.Execute()` and exits with a non-zero status code (typically 1) to signal failure to the shell.
5.  **Database Safety**: Database operations use context-aware transactions to ensure the local state remains consistent even if the process is interrupted.