# Backend Development Guide

## Overview

Strategy Pattern implementation for package format handling. Each backend handles detection, installation, and uninstallation for a specific format.

## Interface

```go
type Backend interface {
    Name() string
    Detect(ctx context.Context, packagePath string) (bool, error)
    Install(ctx context.Context, packagePath string, opts core.InstallOptions, tx *transaction.Manager) (*core.InstallRecord, error)
    Uninstall(ctx context.Context, record *core.InstallRecord) error
}
```

## Registration Order (CRITICAL)

In `backend.go` → `NewRegistryWithDeps()`:

```
1. DEB, RPM      ← Specific archive formats (checked first)
2. AppImage      ← Must precede Binary (AppImages are ELF)
3. Binary        ← Generic ELF executables
4. Tarball/ZIP   ← Generic fallback (checked last)
```

**Wrong order = incorrect detection.** AppImage before Binary is mandatory.

## Adding a New Backend

1. Create `internal/backends/<format>/<format>.go`
2. Embed `base.BaseBackend` for shared deps
3. Implement `Backend` interface
4. Register in `backend.go` at correct priority
5. Add tests with `afero.MemMapFs`

## BaseBackend (Shared Dependencies)

```go
type BaseBackend struct {
    Fs      afero.Fs              // Filesystem abstraction
    Runner  helpers.CommandRunner // Shell command execution
    Paths   paths.Resolver        // System path resolution
    Log     *zerolog.Logger       // Structured logging
    Cfg     *config.Config        // User configuration
}
```

## Transaction Pattern (MANDATORY)

```go
func (b *Backend) Install(..., tx *transaction.Manager) (*core.InstallRecord, error) {
    // ALWAYS register rollback BEFORE the action
    tx.Add("remove_install_dir", func() error {
        return os.RemoveAll(installPath)
    })
    
    // Now safe to create
    if err := os.MkdirAll(installPath, 0755); err != nil {
        return nil, fmt.Errorf("create dir: %w", err)
    }
    
    // Continue with more operations...
}
```

## Detection Rules

- Use **magic numbers** (file signatures), not extensions
- Read first 512 bytes for signature detection
- Return `(true, nil)` only on confident match
- Return `(false, nil)` to pass to next backend
- Return `(false, err)` only on I/O errors

## Where to Look

| Task | File |
|------|------|
| Registry logic | `backend.go` |
| Shared deps struct | `base/base.go` |
| DEB: debtap+pacman | `deb/deb.go` |
| RPM: rpmextract | `rpm/rpm.go` |
| AppImage: squashfs | `appimage/appimage.go` |
| Archives: heuristics | `tarball/tarball.go` |
| ELF binaries | `binary/binary.go` |

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Use file extensions for detection | Check magic numbers |
| Skip `tx.Add()` | Always register rollback first |
| Use `os.` directly | Use injected `Fs` |
| Global logger | Use `b.Log` from BaseBackend |
