# upkg

## Project Overview

**upkg** is a modern, type-safe package manager for Linux written in Go, designed to simplify the installation and management of various package formats including AppImage, DEB, RPM, Tarball, ZIP, and Binary packages. The system implements a Backend Registry Strategy Pattern with transaction management to ensure reliable atomic operations and rollback capabilities.

### Purpose and Main Functionality
- Unified package management across multiple Linux package formats
- Automatic package type detection and backend selection
- Desktop integration with .desktop file generation and Wayland/Hyprland support
- SQLite-based tracking of installed packages with metadata persistence
- Atomic installation operations with rollback capability

### Key Features and Capabilities
- **Multi-format Support**: Handles AppImage, DEB, RPM, Tarball, ZIP, and Binary packages
- **Automatic Detection**: Magic number-based package type identification
- **Desktop Integration**: Generates .desktop files with Wayland environment variable injection
- **Transaction Safety**: Atomic operations with LIFO rollback stack
- **Interactive Management**: CLI with prompts, progress bars, and colored output
- **System Diagnostics**: Built-in doctor command for system health checks
- **Shell Completion**: Generates completion scripts for bash, zsh, fish, and powershell

### Usage Notes
- `upkg install` fails if a target name/path already exists; use `--force` for a clean reinstall on local backends.
- DEB/RPM installs via pacman are treated as system-managed. `doctor` skips their file integrity checks and Hyprland dock icon fix is not attempted for them.
- `upkg doctor` is read-only by default; use `--fix` to create missing directories and verify writability.

### Likely Intended Use Cases
- Linux users who need a unified interface for managing software from different sources
- System administrators who require reliable package installation with rollback capabilities
- Developers who need to test software across multiple package formats
- Users of Wayland/Hyprland desktop environments requiring proper integration

## Table of Contents

- [Architecture](#architecture)
- [C4 Model Architecture](#c4-model-architecture)
- [Repository Structure](#repository-structure)
- [Dependencies and Integration](#dependencies-and-integration)
- [API Documentation](#api-documentation)
- [Development Notes](#development-notes)
- [Known Issues and Limitations](#known-issues-and-limitations)
- [Additional Documentation](#additional-documentation)

## Architecture

### High-level Architecture Overview
The upkg system follows a clean layered architecture with distinct separation of concerns:

- **Presentation Layer**: CLI commands using Cobra framework (`internal/cmd`)
- **Core Domain Layer**: Business logic and data models (`internal/core`)
- **Strategy Layer**: Pluggable backend implementations (`internal/backends`)
- **Infrastructure Layer**: Database, configuration, logging, and utilities

### Technology Stack and Frameworks
- **Language**: Go 1.21+
- **CLI Framework**: Cobra for command-line interface
- **Configuration**: Viper with TOML support
- **Database**: SQLite with modernc.org/sqlite driver
- **Logging**: Zerolog for structured logging
- **File System**: Afero for abstracted file operations
- **UI Libraries**: promptui, tablewriter, progressbar, color

### Component Relationships

```mermaid
graph TD
    subgraph "Presentation Layer"
        CLI[CLI Commands]
    end
    
    subgraph "Core Layer"
        Core[Core Domain Models]
        Transaction[Transaction Manager]
    end
    
    subgraph "Strategy Layer"
        Registry[Backend Registry]
        AppImage[AppImage Backend]
        DEB[DEB Backend]
        RPM[RPM Backend]
        Tarball[Tarball Backend]
        Binary[Binary Backend]
    end
    
    subgraph "Infrastructure Layer"
        DB[(SQLite Database)]
        Config[Configuration]
        Logging[Logging Service]
        Desktop[Desktop Integration]
        Security[Security Validation]
        Heuristics[Heuristics Engine]
    end
    
    CLI </arg_value>
</tool_call>
