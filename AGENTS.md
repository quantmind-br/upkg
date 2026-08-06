# Repository Guidelines

**Generated:** 2026-08-06 | **Commit:** 19f8c18 | **Branch:** main

## Project Overview

`upkg` is a user-space package manager for Linux (Arch-first) supporting DEB, RPM, AppImage, Tarball, ZIP, Binary and Flatpak formats. It detects a package file by content signature, dispatches to a format backend, installs into `~/.local` (`bin/`, `share/applications`, `share/icons/hicolor`, `share/upkg/apps`), records installs in SQLite (`modernc.org/sqlite`), and wraps every mutation in a transaction manager with LIFO rollback and commit finalizers so failures restore the previous state.

Module: `github.com/quantmind-br/upkg` | Go: 1.25.3

## Architecture & Data Flow

**Detection flow:** File → `Registry.DetectBackend()` iterates backends in priority order → first `Detect()` returning true wins → signature check via `helpers.DetectFileType` (first 512 bytes: magic numbers, compressed-tar decompression probe, `ustar` magic at offset 257). Extensions are never trusted.

**Install flow:** CLI command (`internal/cmd/install.go`) → validate flags (timeout first, then path) → `db.New` → `Registry.DetectBackend()` → `transaction.NewManager` with deferred `Rollback` → `Backend.Install()` (each mutation registered via `tx.Add` before it executes) → DB write (`Create` or, for forced local reinstalls, `ReplaceByPath`) → `tx.Commit()` (finalizer failure is warn-only; the install stays) → optional Hyprland dock-icon fix.

**Backend priority:** Flatpak → DEB → RPM → AppImage → Binary → Tarball. AppImage must precede Binary (both are ELF).

**Key interfaces:**

|Interface|Location|Purpose|
|---|---|---|
|`Backend`|`internal/backends/backend.go`|Strategy for package formats: `Name()`, `Detect()`, `Install()`, `Uninstall()`|
|`CommandRunner`|`internal/helpers/exec.go`|Abstracts shell execution for testability (`MockCommandRunner` in `exec_mock.go`)|
|`Provider`|`internal/syspkg/provider.go`|System package manager abstraction (`PacmanProvider` in `syspkg/arch/`)|
|`Scorer`|`internal/heuristics/models.go`|Executable scoring in archives|
|`CacheManager`|`internal/cache/cache.go`|Icon cache + desktop database updates (struct with function fields for mocking)|

**Transaction model** (`internal/transaction/`):

- `Manager.Add(name, RollbackFunc)` — register rollback BEFORE the mutation it reverses.
- `Manager.AddCommit(name, CommitFunc)` — register a commit finalizer (LIFO, run by `Commit()`).
- `Manager.Rollback()` — LIFO; clears both stacks; aggregates errors via `errors.Join`.
- `Manager.Commit() error` — runs finalizers LIFO, clears both stacks; a finalizer failure is returned but does not undo an already-persisted install.
- `Manager.BackupForReplace(fs, target)` (`replacement.go`) — forced-reinstall contract: sibling rename to `.<name>.upkg-backup-<uuid>`, hooks registered before the rename; rollback restores the original, commit drops the backup. No copy-based fallback; rename failure aborts.

**Architecture invariants:**

1. **Transaction safety**: every mutable operation registers rollback via `tx.Add` BEFORE execution; forced replacement uses `BackupForReplace`; DB delete happens only after filesystem removal succeeds (`performUninstall`).
2. **Backend priority**: Flatpak → DEB → RPM → AppImage → Binary → Tarball (order matters).
3. **Signature detection**: file signatures in first 512 bytes, never extensions (`DetectFileType`).
4. **Dependency injection**: all services receive deps via constructors (`NewWithDeps`), never globals.
5. **BaseBackend embedding**: all backends embed `*base.BaseBackend` for shared utilities (Fs, Runner, Paths, Log, Cfg).
6. **Forced reinstall is atomic**: previous install survives replacement failure byte-for-byte; canonical DB row (ID/date) preserved by `ReplaceByPath`.

**Complexity hotspots:**

|File|Lines|Risk|Notes|
|---|---|---|---|
|`internal/backends/tarball/tarball.go`|~890|High|Handles 5+ archive formats, `//nolint:gocyclo`|
|`internal/backends/rpm/rpm.go`|~780|High|Strategy selection + shell execution|
|`internal/icons/icons.go`|~690|Medium|Hardcoded filtering rules grow indefinitely|
|`internal/cmd/uninstall.go`|~540|Medium|Interactive + bulk flows; ad-hoc Flatpak logic (should be backend)|
|`internal/backends/deb/deb.go`|~590|Medium|Debtap + pacman integration|

**Known violations (technical debt):**

|File|Issue|
|---|---|
|`internal/cmd/uninstall.go`|Uses `fmt.Printf` instead of `ui` package (12 call sites)|
|`internal/helpers/archive.go`|Uses direct `os.*` calls instead of `afero.Fs` (15 call sites)|
|`internal/heuristics/scorer.go`|Uses direct `os.Open/Stat`|

## Key Directories

```
upkg/
├── cmd/upkg/main.go         # Entry point (thin wrapper; version via ldflags)
├── internal/
│   ├── backends/            # Strategy pattern: format handlers
│   │   ├── base/            #   BaseBackend (Fs, Runner, Paths, Log, Cfg)
│   │   ├── flatpak/         #   Flatpak (App IDs, .flatpakref)
│   │   ├── deb/             #   DEB (dpkg-deb + debtap) — also debtap.go, dependency.go, icons.go
│   │   ├── rpm/             #   RPM (rpmextract.sh + bsdtar)
│   │   ├── appimage/        #   AppImage (temp exec copy + --appimage-extract, unsquashfs fallback)
│   │   ├── binary/          #   Raw ELF binaries
│   │   └── tarball/         #   Archives (tar.gz/xz/bz2, zip, asar)
│   ├── cmd/                 # Cobra commands: install, uninstall, list, info, doctor, completion, version
│   ├── core/                # Domain models: InstallRecord, InstallOptions, DesktopEntry
│   ├── db/                  # SQLite layer (modernc.org/sqlite), read/write pools, ReplaceByPath
│   ├── transaction/         # LIFO rollback + commit finalizers, BackupForReplace
│   ├── heuristics/          # Executable scoring for archives (Scorer interface)
│   ├── security/            # Path validation, traversal prevention, sanitization
│   ├── helpers/             # CommandRunner, DetectFileType, ShellQuote, archive extraction
│   ├── desktop/             # .desktop parse/write, Wayland env injection
│   ├── icons/               # XDG icon discovery/resizing/filtering
│   ├── syspkg/arch/         # Pacman provider (sudo pacman -U)
│   ├── hyprland/            # Hyprland window client detection
│   ├── ui/                  # Prompts, progress bars, colors
│   ├── cache/               # Icon cache & desktop database updates
│   ├── config/              # TOML via viper (paths, desktop, logging)
│   ├── paths/               # XDG path resolution (Resolver)
│   └── logging/             # Zerolog console + lumberjack rotation
├── pkg-test/                # Sample packages for e2e (gitignored)
├── scripts/e2e-test.sh      # End-to-end test script
└── Makefile                 # Build automation
```

## Development Commands

```bash
make build          # Build to bin/upkg (-ldflags "-X main.version=$(git describe --tags --always --dirty)")
make test           # go test -v -race ./...
make lint           # golangci-lint run ./... (v2, must be on PATH)
make validate       # fmt + vet + lint + test (CI gate)
make quick-check    # fmt + vet + lint (fast pre-commit)
make test-coverage  # coverage.out + coverage.html (atomic mode)
make coverage       # test-coverage + go tool cover -func output
make e2e-test       # scripts/e2e-test.sh with pkg-test fixtures
make run            # Build + execute ./bin/upkg
go test -v -race -run TestName ./path/to/pkg  # Single test
```

## Code Conventions & Common Patterns

### Imports
```go
import (
    "context"                       // 1. Stdlib

    "github.com/rs/zerolog"         // 2. Third-party

    "github.com/quantmind-br/upkg/internal/core"  // 3. Local
)
```

### Error Handling
```go
// ALWAYS wrap with context
return fmt.Errorf("failed to extract package: %w", err)
// Aggregate multi-step failures (uninstall, rollback) with errors.Join
return errors.Join(errs...)
```

### Logging
```go
// CORRECT: injected zerolog from BaseBackend or constructor
b.Log.Info().Str("path", p).Msg("installing")

// FORBIDDEN: fmt.Printf, log.Printf
```

### Filesystem
```go
// ALWAYS afero.Fs (injected via constructor) in backend code
b.Fs.MkdirAll(path, 0755)

// NEVER use os package directly in backends
// Infrastructure packages (config, paths, logging, cache, hyprland) use os — host-level.
```

### Security
```go
// ALWAYS validate user paths before use
if err := security.ValidatePath(userInput); err != nil {
    return err
}
// Archive extraction uses security.ValidateExtractPath (zip-slip prevention)
```

### Transaction Safety
```go
// ALWAYS register rollback BEFORE the mutation
tx.Add("remove_install_dir", func() error {
    return helpers.RemoveIfExists(b.Fs, installPath, true)
})
// Forced replacement: back up existing target, then register restore
if opts.Force {
    if err := tx.BackupForReplace(b.Fs, destPath); err != nil {
        return nil, err
    }
}
// On success path: tx.Commit() drops backups; failure there is warn-only
```

### Config
```go
// viper TOML: ~/.config/upkg/config.toml, UPKG_ env overrides
// Defaults: ~/.local/share/upkg/, info log level, color=auto
```

### Dependency Injection
```go
// Production: New(cfg, log) — real Fs + OS runner
// Testing: NewWithDeps(cfg, log, fs, runner) — afero.MemMapFs/OsFs + MockCommandRunner
// Cache users: NewWithCacheManager(cfg, log, cacheMgr)
```

### Anti-Patterns (Forbidden)

|Pattern|Why|
|---|---|
|`fmt.Printf` for logs|Use zerolog|
|Direct `os.*` filesystem calls in backends|Use injected `afero.Fs`|
|Global state/singletons|Use dependency injection|
|Empty catch blocks|Always handle errors|
|Skipping `tx.Add()` before mutations|Breaks rollback|
|Unvalidated user paths|Security vulnerability|
|Extension-based detection|Use magic numbers (first 512 bytes)|
|Adding generic `.gitignore` rules for test artifacts|Hides regression; fix the leak instead|

## Important Files

|File|Role|
|---|---|
|`cmd/upkg/main.go`|Entry point; wires config → logging → `NewRootCmd`|
|`internal/backends/backend.go`|`Backend` interface + `Registry` (detection, priority order)|
|`internal/backends/base/base.go`|`BaseBackend` struct embedded by all backends|
|`internal/transaction/manager.go`|LIFO rollback stack + commit finalizers|
|`internal/transaction/replacement.go`|`BackupForReplace` — atomic forced reinstall|
|`internal/db/db.go`|SQLite layer; `Create/Get/List/Update/Delete/ReplaceByPath`|
|`internal/cmd/install.go`|Install command; flag validation order, tx lifecycle, `metaKey*` constants|
|`internal/cmd/uninstall.go`|`performUninstall` — DB delete only after successful removal|
|`internal/helpers/detection.go`|`DetectFileType` — signature-based classification|
|`internal/helpers/wrapper.go`|`CreateWrapper`, `ShellQuote`, `IsElectronApp`|
|`internal/helpers/archive.go`|Tar/Zip extraction with bomb limits (10GB/100k files/1000 ratio)|
|`internal/config/config.go`|TOML config via viper|
|`internal/paths/paths.go`|XDG resolution; `NewResolverWithHome` for test isolation|

## Runtime/Tooling Preferences

- **Go version**: 1.25.3
- **Build**: `go build -ldflags "-X main.version=$(git describe --tags --always --dirty)" -o bin/upkg ./cmd/upkg`
- **Formatter**: `gofmt -s -w .`
- **Linter**: golangci-lint v2 (`.golangci.yml` with `version: "2"`; on PATH, not vendored). Enabled: goconst, gocyclo (min-complexity 15), gosec (G204 excluded), misspell, revive, unconvert, unparam. Test files exclude errcheck/goconst/gocyclo/gosec/revive/unparam.
- **Race detector**: enabled for all test targets (`-race`)
- **Coverage**: atomic mode, HTML via `go tool cover`

**External dependencies (host):**

|Category|Tools|
|---|---|
|AppImage|`unsquashfs` (fallback), `desktop-file-validate` (warn-only)|
|Debian|`dpkg-deb`, `debtap`, `pacman` (Arch-only backend)|
|RPM|`rpmextract.sh` (preferred), `bsdtar` (fallback)|
|Flatpak|`flatpak` (`install`, `list --user --app`, `uninstall --user --noninteractive -y`)|
|Desktop caches|`gtk4-update-icon-cache` (falls back to `gtk-update-icon-cache`), `update-desktop-database` (sudo-wrapped when target needs it)|
|Compositor|`hyprctl`|
|Misc|`sudo`; `tar`/`bash` only in tests/e2e, not runtime (extraction is pure-Go `archive/tar`)|

## Testing & QA

- **Framework**: `testify/assert`, `testify/require`
- **Filesystem**: `afero.NewMemMapFs()` for pure-fs logic (wrapper quoting, detection, transaction); `afero.NewOsFs()` for install/extraction paths (real binaries, ELF, chmod); error-injection wrappers (`errorFs`, `binDirFailFs`, `renameErrorFs`) fail specific ops with `assert.AnError`.
- **Commands**: `helpers.MockCommandRunner` — 10 optional Func fields, nil-safe defaults.
- **Cache**: `cache.MockCacheManager` with function fields.
- **Home isolation (BUG-010)**: never `os.Setenv("HOME")`. Each test does `home := t.TempDir()` and `backend.Paths = paths.NewResolverWithHome(cfg, home)`; empty string injects a "no home" failure. Tests run `t.Parallel()` freely with this pattern.
- **Pattern**: table-driven tests with `t.Run`, per-package white-box tests (`package appimage`, not `_test`).
- **Regression patterns to copy**: `TestAppImageBackend_Install_WithForce` (force install replaces bytes, `tx.Rollback()` restores previous bytes, no `upkg-backup-*` remnants); `transaction/replacement_test.go` suite; `db/db_test.go` `ReplaceByPath` suite; `internal/cmd/install_test.go` + `uninstall_test.go` timeout tables; `helpers/detection_test.go` signature suite (renamed extensions stay unknown); `helpers/wrapper_test.go` ShellQuote table.
- **Fixtures**: real archives built in Go at runtime (`makeTarGz`, `makeZip`, `makeTarXz` in `test_helpers_test.go`/`detect_helpers_test.go`; `makeTarGzWithELF` packs `os.Executable()` so the scorer accepts it). `pkg-test/` holds real packages for e2e.
- **Skips**: only environment-gated — `testing.Short`, missing external binary (`bash`, `tar`, `desktop-file-validate`), unsupported symlink/permissions, missing `pkg-test` fixtures. Permanent "requires mocking" stubs are violations (some legacy ones remain in `deb_extra_test.go`/rpm — do not add more).
- **E2E**: `make e2e-test` runs `scripts/e2e-test.sh` — builds binary if missing, temp HOME, tests AppImage/binary/tarball/zip always; deb/rpm gated behind `UPKG_E2E_SYSTEM=1`. Results logged to `test-results/baseline.txt`.
- **CI gate**: `make validate` (fmt + vet + lint + test).
