# upkg Agent Guidelines

**Generated:** 2026-01-19 | **Commit:** df60c25 | **Branch:** main

## Overview

Go-based Linux package manager supporting DEB, RPM, AppImage, Tarball, ZIP, Binary formats. Core architecture: Backend Registry (Strategy Pattern) + Transaction Manager (atomic operations with LIFO rollback).

## Structure

```
upkg/
├── cmd/upkg/main.go      # Entry point (thin wrapper)
├── internal/
│   ├── backends/         # Strategy pattern: format handlers (see backends/AGENTS.md)
│   ├── cmd/              # CLI commands via Cobra (see cmd/AGENTS.md)
│   ├── core/             # Domain models: InstallRecord, InstallOptions
│   ├── db/               # SQLite layer (modernc.org/sqlite)
│   ├── transaction/      # Atomic ops with LIFO rollback stack
│   ├── heuristics/       # Executable scoring for archives
│   ├── security/         # Path validation, traversal prevention
│   ├── helpers/          # Command execution, archive handling
│   ├── desktop/          # .desktop file generation
│   ├── icons/            # XDG icon discovery/resizing
│   └── ui/               # Progress bars, prompts, colors
├── pkg-test/             # Sample packages for testing
└── Makefile              # Build automation
```

## Where to Look

| Task | Location | Notes |
|------|----------|-------|
| Add package format | `internal/backends/<format>/` | Implement `Backend` interface |
| Add CLI command | `internal/cmd/<name>.go` | Factory pattern, register in `root.go` |
| Modify install flow | `internal/backends/` + `internal/transaction/` | Always use tx.Add() |
| Fix icon detection | `internal/icons/icons.go` | XDG-compliant logic |
| Archive heuristics | `internal/heuristics/scorer.go` | Executable scoring |
| Path security | `internal/security/` | Validate ALL user paths |
| Database queries | `internal/db/db.go` | JSON metadata storage |

## Commands

```bash
make build          # Build to bin/upkg (with version ldflags)
make test           # Run all tests with -race
make lint           # golangci-lint (complexity max 15)
make validate       # fmt + vet + lint + test (CI gate)
make test-coverage  # Generate coverage.html
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
// CORRECT: Use injected zerolog
log.Info().Str("path", p).Msg("installing")

// FORBIDDEN: fmt.Printf, log.Printf
```

### Filesystem
```go
// ALWAYS use afero.Fs (injected via constructor)
// NEVER use os package directly in production code
```

### Security
```go
// ALWAYS validate user paths
if err := security.ValidatePath(userInput); err != nil {
    return err
}
```

## Anti-Patterns (Forbidden)

| Pattern | Why |
|---------|-----|
| `fmt.Printf` for logs | Use zerolog |
| Direct `os.` filesystem calls | Use injected `afero.Fs` |
| Global state/singletons | Use dependency injection |
| Empty `catch(e) {}` | Always handle errors |
| Skipping `tx.Add()` before mutations | Breaks rollback |
| Unvalidated user paths | Security vulnerability |

## Testing

- **Framework**: `testify/assert`, `testify/require`
- **Filesystem**: Always mock with `afero.NewMemMapFs()`
- **Commands**: Use `helpers.MockCommandRunner`
- **Pattern**: Table-driven tests with `t.Run()`
- **Co-location**: `*_test.go` next to source

## Architecture Invariants

1. **Transaction Safety**: Every mutable operation registers rollback via `tx.Add(name, func)` BEFORE execution
2. **Backend Priority**: DEB/RPM -> AppImage -> Binary -> Tarball (order matters for detection)
3. **Magic Number Detection**: Use file signatures, not extensions
4. **Dependency Injection**: All services receive deps via constructors, never globals

## Complexity Hotspots

| File | Lines | Risk |
|------|-------|------|
| `backends/deb/deb.go` | 1286 | God object: debtap + pacman + icons |
| `backends/tarball/tarball.go` | 931 | ASAR parsing + heuristics |
| `backends/rpm/rpm.go` | 904 | Format-specific integration |
| `icons/icons.go` | 683 | XDG filtering logic |

## External Dependencies

**Required:** `tar`, `unsquashfs`, `bsdtar`, `dpkg-deb`, `rpm`
**Arch-specific:** `debtap`, `pacman`, `rpmextract.sh`
**Desktop:** `gtk4-update-icon-cache`, `update-desktop-database`
