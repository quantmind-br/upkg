# Data Flow Analysis

## Data Models

The system architecture centers around the lifecycle of a software package installation, represented by several core Go structures:

*   **`InstallRecord`**: The primary data structure tracking a successful installation. It includes the `InstallID` (generated hash), `PackageType` (deb, rpm, appimage, etc.), file system paths (`InstallPath`, `DesktopFile`), and timestamps.
*   **`Metadata`**: Embedded within `InstallRecord`, it stores package-specific details like icon file paths, wrapper scripts, Wayland support levels, and original desktop file references.
*   **`DesktopEntry`**: A structured representation of Linux `.desktop` files, mapping INI-style keys (Name, Exec, Icon, etc.) to internal fields for manipulation (e.g., Wayland environment injection).
*   **`IconFile`**: Represents discovered assets with metadata for size, extension, and absolute path, used during the icon resolution phase.
*   **`Install` (Database Model)**: A flat version of the install record optimized for SQLite storage, where complex metadata is serialized as a JSON string.

## Input Sources

Data enters the system through multiple channels:

*   **CLI Arguments**: Paths to package files (`.AppImage`, `.deb`, `.rpm`, `.tar.gz`) and flags for installation options (force, custom names, timeout).
*   **Package Files (Binary Blobs)**: Raw package data read from the filesystem, which is then extracted or parsed by specific backends.
*   **System Environment**: Detection of the running environment (e.g., checking if Hyprland is active, determining HOME directories).
*   **Filesystem Metadata**: Reading existing `.desktop` files, searching for icon assets in standard paths (`/usr/share/icons`, `~/.local/share/icons`), and inspecting binary ELF headers for type detection.

## Data Transformations

Information undergoes several stages of processing and enrichment:

1.  **Type Detection**: A raw package path is transformed into a concrete `Backend` implementation (e.g., `AppImageBackend`) through file extension checks and magic byte analysis.
2.  **Extraction & Parsing**: Packages are extracted to temporary directories. Metadata is pulled from internal structures (like DEB control files or AppImage squashfs roots) and mapped into `ExtractedMetadata`.
3.  **Path Normalization**: User-provided names are sanitized and normalized (e.g., `AppName` -> `app-name`) to create predictable filesystem paths and IDs.
4.  **Desktop File Enrichment**: Original `.desktop` files are parsed, validated, and modified. A key transformation is the injection of Wayland-specific environment variables (`GDK_BACKEND`, `QT_QPA_PLATFORM`) into the `Exec` line.
5.  **Icon Resolution**: The system scans for high-resolution icons within extracted packages and maps them to standard system icon paths.
6.  **Serialization**: The high-level `InstallRecord` Go struct is converted into a `db.Install` struct, with the `Metadata` field being marshaled into a JSON string for database persistence.

## Storage Mechanisms

The system employs a multi-layered storage strategy:

*   **SQLite Database**: A persistent store for installation records. It uses a Write-Ahead Logging (WAL) mode for concurrency and maintains separate read/write connection pools. It also tracks schema versions via a `schema_migrations` table.
*   **Local Filesystem**: 
    *   `~/.local/bin/`: Binaries and AppImages are moved or symlinked here.
    *   `~/.local/share/applications/`: Generated and modified `.desktop` files.
    *   `~/.local/share/icons/`: Extracted and resolved icons.
    *   `~/.local/share/upkg/`: Package-specific installation data.
*   **Temporary Storage**: Uses `afero.Fs` (abstraction over OS filesystem) to create temporary directories for package extraction and intermediate processing.

## Data Validation

Validation is strictly enforced at several boundary points:

*   **Security Validation**: Paths are checked for "dangerous" patterns (path traversal, command injection characters) and sensitive system areas (e.g., `/etc`, `/boot`).
*   **Package Name Sanitization**: Custom names and extracted names are sanitized to prevent illegal characters in filenames and shell commands.
*   **Desktop Entry Validation**: Required fields (Type, Name, Exec) are verified before a `.desktop` file is written to the system.
*   **Transaction Integrity**: A `Manager` tracks filesystem changes (created files, modified directories) and performs a rollback if any step in the installation/database-save pipeline fails.

## Output Formats

The system produces several types of output:

*   **CLI UI**: Structured, color-coded status messages (using `fatih/color`) and progress bars indicating detection, installation progress, and success/failure.
*   **Structured Logs**: ZeroLog-formatted JSON logs capturing detailed internal events and error contexts.
*   **System Integration**: Standard Linux integration files (.desktop entries, icons, binaries) that allow the installed software to be recognized by desktop environments and application launchers.
*   **Return Codes**: Standardized exit codes (0 for success, 1-8 and 130 for specific error categories) for shell integration.