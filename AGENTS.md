# upkg Agent Guidelines

**Generated:** 2026-01-21 | **Commit:** 028c695 | **Branch:** main

## Overview

Go-based Linux package manager supporting DEB, RPM, AppImage, Tarball, ZIP, Binary, Flatpak formats. Core architecture: Backend Registry (Strategy Pattern) + Transaction Manager (atomic operations with LIFO rollback).

## Structure

```
upkg/
├── cmd/upkg/main.go      # Entry point (thin wrapper)
├── internal/
│   ├── backends/         # Strategy pattern: format handlers (see backends/AGENTS.md)
│   ├── cmd/              # CLI commands via Cobra (see cmd/AGENTS.md)
│   ├── core/             # Domain models: InstallRecord, Metadata, DesktopEntry
│   ├── db/               # SQLite layer (modernc.org/sqlite), read/write pools
│   ├── transaction/      # Atomic ops with LIFO rollback stack
│   ├── heuristics/       # Executable scoring for archives (Scorer interface)
│   ├── security/         # Path validation, traversal prevention, sanitization
│   ├── helpers/          # Command execution (CommandRunner), archive handling
│   ├── desktop/          # .desktop file generation
│   ├── icons/            # XDG icon discovery/resizing/filtering
│   ├── syspkg/           # System package manager abstraction (Provider interface)
│   ├── hyprland/         # Hyprland compositor integration
│   └── ui/               # Progress bars, prompts, colors
├── pkg-test/             # Sample packages for integration testing
└── Makefile              # Build automation
```

## Where to Look

| Task | Location | Notes |
|------|----------|-------|
| Add package format | `internal/backends/<format>/` | Implement `Backend` interface, register in `backend.go` |
| Add CLI command | `internal/cmd/<name>.go` | Factory pattern, register in `root.go` |
| Modify install flow | `internal/backends/` + `internal/transaction/` | Always use `tx.Add()` BEFORE mutation |
| Fix icon detection | `internal/icons/icons.go` | XDG-compliant filtering logic |
| Archive heuristics | `internal/heuristics/scorer.go` | `Scorer` interface, `ChooseBest` method |
| Path security | `internal/security/validation.go` | `ValidateFilePath`, `ValidateExtractPath` |
| Database queries | `internal/db/db.go` | JSON metadata storage, migrations in `db/migrations/` |
| System pkg manager | `internal/syspkg/` | `Provider` interface, Arch impl in `arch/` |

## Key Interfaces

| Interface | Location | Purpose |
|-----------|----------|---------|
| `Backend` | `backends/backend.go` | Strategy for package formats (Detect/Install/Uninstall) |
| `CommandRunner` | `helpers/exec.go` | Abstracts shell execution for testability |
| `Provider` | `syspkg/provider.go` | System package manager abstraction |
| `Scorer` | `heuristics/models.go` | Executable scoring in archives |

## Commands

```bash
make build          # Build to bin/upkg (with version ldflags)
make test           # Run all tests with -race
make lint           # golangci-lint (complexity max 15)
make validate       # fmt + vet + lint + test (CI gate)
make test-coverage  # Generate coverage.html
make e2e-test       # Run scripts/e2e-test.sh with pkg-test fixtures
go test -v -race -run TestName ./path/to/pkg  # Single test
```

## Conventions

### Imports
```go
import (
    "context"           // 1. Stdlib
    
    "github.com/rs/zerolog"  // 2. Third-party
    
    "upkg/internal/core"     // 3. Local
)
```

### Error Handling
```go
// ALWAYS wrap with context
return fmt.Errorf("failed to extract package: %w", err)
```

### Logging
```go
// CORRECT: Use injected zerolog from BaseBackend
b.Log.Info().Str("path", p).Msg("installing")

// FORBIDDEN: fmt.Printf, log.Printf
```

### Filesystem
```go
// ALWAYS use afero.Fs (injected via constructor)
b.Fs.MkdirAll(path, 0755)

// NEVER use os package directly in production code
```

### Security
```go
// ALWAYS validate user paths before use
if err := security.ValidatePath(userInput); err != nil {
    return err
}
```

### Transaction Safety
```go
// ALWAYS register rollback BEFORE the mutation
tx.Add("remove_install_dir", func() error {
    return b.Fs.RemoveAll(installPath)
})
// Now safe to create
if err := b.Fs.MkdirAll(installPath, 0755); err != nil {
    return nil, err
}
```

## Anti-Patterns (Forbidden)

| Pattern | Why |
|---------|-----|
| `fmt.Printf` for logs | Use zerolog |
| Direct `os.*` filesystem calls | Use injected `afero.Fs` |
| Global state/singletons | Use dependency injection |
| Empty catch blocks | Always handle errors |
| Skipping `tx.Add()` before mutations | Breaks rollback |
| Unvalidated user paths | Security vulnerability |
| File extension detection | Use magic numbers (file signatures) |

## Testing

- **Framework**: `testify/assert`, `testify/require`
- **Filesystem**: Always mock with `afero.NewMemMapFs()`
- **Commands**: Use `helpers.MockCommandRunner`
- **Pattern**: Table-driven tests with `t.Run()`, always `t.Parallel()`
- **Co-location**: `*_test.go` next to source
- **Fixtures**: Real packages in `pkg-test/` for integration tests

## Architecture Invariants

1. **Transaction Safety**: Every mutable operation registers rollback via `tx.Add(name, func)` BEFORE execution
2. **Backend Priority**: Flatpak -> DEB/RPM -> AppImage -> Binary -> Tarball (order matters for detection)
3. **Magic Number Detection**: Use file signatures in first 512 bytes, not extensions
4. **Dependency Injection**: All services receive deps via constructors (`NewWithDeps`), never globals
5. **BaseBackend Embedding**: All backends embed `*backendbase.BaseBackend` for shared utilities

## Complexity Hotspots

| File | Lines | Risk | Notes |
|------|-------|------|-------|
| `backends/rpm/rpm.go` | 900 | High | God object: mixes strategy selection + shell execution |
| `backends/tarball/tarball.go` | 875 | High | Handles 5+ archive formats, has `//nolint:gocyclo` |
| `icons/icons.go` | 683 | Medium | Hardcoded filtering rules grow indefinitely |
| `cmd/uninstall.go` | 531 | Medium | Contains ad-hoc Flatpak logic (should be backend) |
| `backends/deb/deb.go` | 588 | Medium | Debtap + pacman integration |

## Known Violations (Technical Debt)

| File | Issue |
|------|-------|
| `internal/cmd/uninstall.go` | Uses `fmt.Printf` instead of `ui` package |
| `internal/helpers/archive.go` | Uses direct `os.*` calls instead of `afero.Fs` |
| `internal/heuristics/scorer.go` | Uses direct `os.Open/Stat` |

## External Dependencies

**Required:** `tar`, `unsquashfs`, `bsdtar`, `dpkg-deb`, `rpm`
**Arch-specific:** `debtap`, `pacman`, `rpmextract.sh`
**Desktop:** `gtk4-update-icon-cache`, `update-desktop-database`
