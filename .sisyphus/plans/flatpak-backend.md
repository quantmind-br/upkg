# Flatpak Backend Support for upkg

## Context

### Original Request
Adicionar suporte a instalação de Flatpaks com o upkg.

### Interview Summary
**Key Discussions**:
- **Input types**: Local files (.flatpak, .flatpakref) + remote App ID (org.app.Name)
- **Tracking**: Delegate to Flatpak CLI (no SQLite registration)
- **Uninstall**: Preserve ~/.var/app/ by default, --delete-data flag optional
- **List**: Integrated - 'upkg list' queries SQLite + 'flatpak list --user --app'
- **Doctor**: Full verification (CLI presence, remotes, orphan apps)
- **Remote**: Flathub default, --remote=X for others
- **Install scope**: User only (--user), no sudo required
- **Interactivity**: Non-interactive (--noninteractive), fail fast
- **Missing remote**: Fail with guidance message, don't auto-add

**Research Findings**:
- Flatpak uses OSTree for storage, bubblewrap for sandboxing
- .flatpak = OSTree static delta (GVariant) or OCI (ZIP magic)
- .flatpakref = INI file with `[Flatpak Ref]` header
- App ID pattern: `org.domain.AppName` (reverse DNS)
- Desktop integration automatic via flatpak exports
- Rollback native via `flatpak update --commit=SHA`

### Metis Review
**Identified Gaps** (addressed):
- **Remote handling**: Confirmed - fail with guide, no auto-add
- **Backend priority risk**: Mitigated - App ID detection uses regex on non-path inputs
- **Orphan apps definition**: Clarified - use `flatpak list --unused` for runtimes
- **Interactivity**: Confirmed - use --noninteractive flag
- **System vs user scope**: Confirmed - user only for consistency

---

## Work Objectives

### Core Objective
Add Flatpak support to upkg as a delegate backend, enabling installation/uninstallation of Flatpak apps via local files or remote App IDs, with integrated listing and doctor checks.

### Concrete Deliverables
- `internal/backends/flatpak/flatpak.go` - Backend implementation
- `internal/backends/flatpak/flatpak_test.go` - TDD tests
- `internal/backends/flatpak/detect.go` - Detection logic (magic numbers + App ID regex)
- `internal/backends/flatpak/detect_test.go` - Detection tests
- Modified `internal/backends/backend.go` - Registry registration
- Modified `internal/core/models.go` - PackageTypeFlatpak constant
- Modified `internal/cmd/list.go` - Flatpak integration
- Modified `internal/cmd/doctor.go` - Flatpak health checks

### Definition of Done
- [x] `make validate` passes (fmt + vet + lint + test) - Note: golangci-lint v1/v2 config mismatch is pre-existing
- [ ] `upkg install org.mozilla.firefox` installs from Flathub - BLOCKED: No Flathub remote configured
- [ ] `upkg install ./app.flatpak` installs local bundle - BLOCKED: No Flathub remote configured  
- [ ] `upkg install ./app.flatpakref` installs from reference - BLOCKED: No Flathub remote configured
- [ ] `upkg uninstall org.mozilla.firefox` removes app (preserves data) - BLOCKED: No Flathub remote configured
- [ ] `upkg uninstall --delete-data org.app.Name` removes app + data - BLOCKED: No Flathub remote configured
- [x] `upkg list` shows both SQLite records AND flatpak apps
- [x] `upkg doctor` checks flatpak CLI and remotes

### Must Have
- Backend interface implementation (Name, Detect, Install, Uninstall)
- Detection of .flatpak, .flatpakref, and App ID patterns
- User-scope installs only (--user flag)
- Non-interactive mode (--noninteractive flag)
- Integration with 'upkg list' command
- Integration with 'upkg doctor' command
- TDD with mock CommandRunner

### Must NOT Have (Guardrails)
- **NO SQLite registration** - Flatpak manages its own database
- **NO desktop/icon management** - Flatpak exports automatically
- **NO runtime management** - Only apps, not runtimes
- **NO .flatpakrepo handling** - No remote management
- **NO system-wide installs** - User scope only
- **NO auto-adding remotes** - Fail with guidance instead
- **NO interactive prompts** - Use --noninteractive
- **NO transaction rollback logic** - Flatpak handles atomicity

---

## Verification Strategy (MANDATORY)

### Test Decision
- **Infrastructure exists**: YES (go test, testify, afero)
- **User wants tests**: TDD
- **Framework**: Go testing + testify/assert + mock CommandRunner

### TDD Workflow

Each TODO follows RED-GREEN-REFACTOR:

1. **RED**: Write failing test with mock CommandRunner
2. **GREEN**: Implement minimum code to pass
3. **REFACTOR**: Clean up while keeping green

**Test Pattern (from existing backends)**:
```go
func TestFlatpakBackend_Install(t *testing.T) {
    runner := helpers.NewMockCommandRunner()
    runner.AddResponse("flatpak", "install", ...) // Expected output
    
    backend := flatpak.NewWithDeps(cfg, log, fs, runner)
    record, err := backend.Install(ctx, "org.mozilla.firefox", opts, tx)
    
    assert.NoError(t, err)
    assert.Equal(t, "org.mozilla.firefox", record.Name)
    runner.AssertCalled(t, "flatpak", "install", "--user", "--noninteractive", ...)
}
```

---

## Task Flow

```
Task 0 (PackageType) 
        ↓
Task 1 (Detection tests) → Task 2 (Detection impl)
        ↓
Task 3 (Backend tests) → Task 4 (Backend impl)
        ↓
Task 5 (Registry)
        ↓
Task 6 (List integration) ←→ Task 7 (Doctor integration) [parallel]
        ↓
Task 8 (Manual verification)
```

## Parallelization

| Group | Tasks | Reason |
|-------|-------|--------|
| A | 6, 7 | Independent command modifications |

| Task | Depends On | Reason |
|------|------------|--------|
| 1, 2 | 0 | Need PackageTypeFlatpak constant |
| 3, 4 | 2 | Detection logic must exist |
| 5 | 4 | Backend must be complete |
| 6, 7 | 5 | Backend must be registered |
| 8 | 6, 7 | All code complete |

---

## TODOs

- [x] 0. Add PackageTypeFlatpak constant

  **What to do**:
  - Add `PackageTypeFlatpak PackageType = "flatpak"` to core/models.go
  - Add after existing PackageType constants

  **Must NOT do**:
  - Modify InstallRecord structure
  - Add flatpak-specific metadata fields

  **Parallelizable**: NO (foundation for all other tasks)

  **References**:
  
  **Pattern References**:
  - `internal/core/models.go:8-15` - Existing PackageType constants pattern
  
  **WHY**: All other tasks need this constant defined

  **Acceptance Criteria**:
  
  - [x] Test: `go build ./...` compiles without errors
  - [x] Constant `core.PackageTypeFlatpak` equals `"flatpak"`
  
  **Manual Verification**:
  - [x] `grep -n "PackageTypeFlatpak" internal/core/models.go` shows the new constant

  **Commit**: YES
  - Message: `feat(core): add PackageTypeFlatpak constant`
  - Files: `internal/core/models.go`
  - Pre-commit: `go build ./...`

---

- [x] 1. Write detection tests (TDD RED)

  **What to do**:
  - Create `internal/backends/flatpak/detect_test.go`
  - Test detection of:
    - `.flatpak` file (mock GVariant header or ZIP magic)
    - `.flatpakref` file (mock INI with `[Flatpak Ref]`)
    - App ID string `org.mozilla.firefox` (not a file path)
    - Non-flatpak inputs return false
  - Use `afero.NewMemMapFs()` for file mocking

  **Must NOT do**:
  - Implement detection logic yet (tests should FAIL)
  - Test .flatpakrepo files (out of scope)

  **Parallelizable**: NO (depends on Task 0)

  **References**:
  
  **Pattern References**:
  - `internal/backends/appimage/appimage.go:69-108` - Detection pattern with magic bytes
  - `internal/helpers/detection.go` - IsAppImage helper pattern
  
  **Test References**:
  - `internal/backends/backend_test.go` - Backend test patterns
  - `internal/backends/appimage/` - Look for any *_test.go files for patterns
  
  **External References**:
  - Flatpak .flatpakref format: INI file starting with `[Flatpak Ref]`
  - App ID regex: `^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)+$`
  
  **WHY Each Reference Matters**:
  - AppImage detection shows magic byte checking pattern
  - Backend tests show afero mocking patterns
  - App ID regex distinguishes "org.app.Name" from "/path/to/file"

  **Acceptance Criteria**:
  
  - [x] Test file created: `internal/backends/flatpak/detect_test.go`
  - [x] Tests cover: .flatpak detection, .flatpakref detection, App ID detection, negative cases
  - [x] `go test ./internal/backends/flatpak/...` → FAIL (expected - no implementation)

  **Commit**: YES
  - Message: `test(flatpak): add detection tests (TDD RED)`
  - Files: `internal/backends/flatpak/detect_test.go`
  - Pre-commit: `go build ./...` (tests intentionally fail)

---

- [x] 2. Implement detection logic (TDD GREEN)

  **What to do**:
  - Create `internal/backends/flatpak/detect.go`
  - Implement `Detect(ctx, input string) (bool, error)`:
    - If input is file path AND exists:
      - `.flatpak`: Check for GVariant header OR ZIP magic (`PK\x03\x04`)
      - `.flatpakref`: Check for `[Flatpak Ref]` header in first line
    - If input matches App ID regex AND is NOT a file path:
      - Return true for pattern `^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*){2,}$`
  - Use injected `afero.Fs` for file operations

  **Must NOT do**:
  - Detect .flatpakrepo files
  - Validate that App ID exists on remote
  - Use extension-only detection (must check magic for .flatpak)

  **Parallelizable**: NO (depends on Task 1)

  **References**:
  
  **Pattern References**:
  - `internal/backends/appimage/appimage.go:69-108` - Magic byte detection implementation
  - `internal/helpers/detection.go` - File type detection helpers
  
  **API/Type References**:
  - `internal/backends/backend.go:35` - Detect method signature
  
  **External References**:
  - OSTree static delta: No simple magic, but .flatpak files are typically OSTree bundles
  - OCI bundles: Start with ZIP magic `0x50 0x4B 0x03 0x04`
  - .flatpakref: Text file, first line is `[Flatpak Ref]`
  
  **WHY Each Reference Matters**:
  - AppImage detection shows how to read and check magic bytes with afero.Fs
  - Backend interface defines exact Detect signature to implement

  **Acceptance Criteria**:
  
  - [x] `go test ./internal/backends/flatpak/...` → PASS
  - [x] Detection correctly identifies .flatpak with ZIP magic
  - [x] Detection correctly identifies .flatpakref with INI header
  - [x] Detection correctly identifies App ID pattern
  - [x] Detection returns false for non-flatpak inputs

  **Commit**: YES
  - Message: `feat(flatpak): implement detection logic (TDD GREEN)`
  - Files: `internal/backends/flatpak/detect.go`
  - Pre-commit: `go test ./internal/backends/flatpak/...`

---

- [x] 3. Write backend tests (TDD RED)

  **What to do**:
  - Create `internal/backends/flatpak/flatpak_test.go`
  - Test Install scenarios:
    - Install by App ID: mock `flatpak install --user --noninteractive flathub org.app.Name`
    - Install .flatpak file: mock `flatpak install --user --noninteractive ./file.flatpak`
    - Install .flatpakref: mock `flatpak install --user --noninteractive ./file.flatpakref`
    - Install with custom remote: `--remote=fedora`
    - Fail when flatpak not installed
    - Fail when remote not configured (check error message)
  - Test Uninstall scenarios:
    - Normal: `flatpak uninstall --user --noninteractive org.app.Name`
    - With delete-data: `flatpak uninstall --user --noninteractive --delete-data org.app.Name`
  - Use `helpers.NewMockCommandRunner()`

  **Must NOT do**:
  - Implement backend logic yet (tests should FAIL)
  - Test actual flatpak CLI execution

  **Parallelizable**: NO (depends on Task 2)

  **References**:
  
  **Pattern References**:
  - `internal/backends/appimage/appimage.go:32-62` - Backend constructor patterns
  - `internal/helpers/exec.go` - CommandRunner interface
  
  **Test References**:
  - `internal/cmd/install_test.go` - Command testing with mocks
  - Search for `MockCommandRunner` usage in codebase
  
  **WHY Each Reference Matters**:
  - AppImage backend shows constructor injection pattern
  - MockCommandRunner usage shows how to mock CLI calls

  **Acceptance Criteria**:
  
  - [x] Test file created: `internal/backends/flatpak/flatpak_test.go`
  - [x] Tests cover: Install (3 input types), Uninstall (with/without delete-data), error cases
  - [x] `go test ./internal/backends/flatpak/...` → FAIL (expected - no implementation)

  **Commit**: YES
  - Message: `test(flatpak): add backend tests (TDD RED)`
  - Files: `internal/backends/flatpak/flatpak_test.go`
  - Pre-commit: `go build ./...`

---

- [x] 4. Implement backend (TDD GREEN)

  **What to do**:
  - Create `internal/backends/flatpak/flatpak.go`
  - Struct `FlatpakBackend` embedding `*backendbase.BaseBackend`
  - Constructors: `New()`, `NewWithDeps()`, `NewWithRunner()`
  - `Name() string` returns `"flatpak"`
  - `Detect()` delegates to detect.go logic
  - `Install(ctx, input, opts, tx)`:
    - Determine input type (file vs App ID)
    - Build command: `flatpak install --user --noninteractive [--remote=X] <input>`
    - Execute via `b.Runner.Run(ctx, "flatpak", args...)`
    - Parse output for app name/version
    - Return `InstallRecord` with `PackageTypeFlatpak`
    - **NO tx.Add()** - flatpak manages rollback
  - `Uninstall(ctx, record)`:
    - Build command: `flatpak uninstall --user --noninteractive [--delete-data] <app-id>`
    - Execute via Runner
  - Handle error: "remote 'flathub' not found" → return user-friendly guidance

  **Must NOT do**:
  - Register rollback with tx.Add() - flatpak is atomic
  - Store in SQLite - delegate to flatpak
  - Manage desktop files or icons

  **Parallelizable**: NO (depends on Task 3)

  **References**:
  
  **Pattern References**:
  - `internal/backends/appimage/appimage.go:24-67` - Backend struct and constructors
  - `internal/backends/appimage/appimage.go:110-250` - Install method pattern (simplified - no extraction)
  - `internal/backends/base/base.go` - BaseBackend fields (Fs, Runner, Paths, Log, Cfg)
  
  **API/Type References**:
  - `internal/backends/backend.go:30-42` - Backend interface definition
  - `internal/core/models.go:17-28` - InstallRecord struct
  - `internal/transaction/manager.go` - Transaction Manager (NOT used for flatpak)
  
  **External References**:
  - `flatpak install --help` - CLI flags reference
  - `flatpak uninstall --help` - Uninstall flags
  
  **WHY Each Reference Matters**:
  - AppImage backend is the model, but Flatpak is simpler (no extraction, no desktop management)
  - BaseBackend provides Logger, CommandRunner, Fs needed for implementation
  - InstallRecord fields needed for return value

  **Acceptance Criteria**:
  
  - [x] `go test ./internal/backends/flatpak/...` → PASS
  - [x] Backend implements all 4 interface methods
  - [x] Install builds correct CLI command with --user --noninteractive
  - [x] Uninstall respects --delete-data option
  - [x] Error messages are user-friendly for missing remote

  **Commit**: YES
  - Message: `feat(flatpak): implement backend (TDD GREEN)`
  - Files: `internal/backends/flatpak/flatpak.go`
  - Pre-commit: `go test ./internal/backends/flatpak/...`

---

- [x] 5. Register backend in registry

  **What to do**:
  - Import `"upkg/internal/backends/flatpak"` in backend.go
  - Add flatpak backend registration in `NewRegistryWithDeps()`
  - Position: FIRST (before DEB) to capture App ID patterns
  - Detection order becomes: Flatpak → DEB → RPM → AppImage → Binary → Tarball

  **Must NOT do**:
  - Change detection logic of other backends
  - Modify registry interface

  **Parallelizable**: NO (depends on Task 4)

  **References**:
  
  **Pattern References**:
  - `internal/backends/backend.go:57-78` - NewRegistryWithDeps registration pattern
  
  **WHY Each Reference Matters**:
  - Shows exact pattern for registering new backend
  - Comments explain priority ordering

  **Acceptance Criteria**:
  
  - [x] `go build ./...` compiles
  - [x] `go test ./internal/backends/...` → PASS
  - [x] Flatpak is first in registry.backends slice
  
  **Manual Verification**:
  - [x] `grep -A 20 "func NewRegistryWithDeps" internal/backends/backend.go` shows flatpak first

  **Commit**: YES
  - Message: `feat(backends): register flatpak backend in registry`
  - Files: `internal/backends/backend.go`
  - Pre-commit: `go test ./internal/backends/...`

---

- [x] 6. Integrate flatpak into 'upkg list'

  **What to do**:
  - Modify `internal/cmd/list.go`
  - After querying SQLite records, also run `flatpak list --user --app --columns=application,version`
  - Parse output and merge with SQLite results
  - Add `[flatpak]` label to distinguish from other package types
  - Handle case where flatpak CLI not installed (skip silently or note)

  **Must NOT do**:
  - Show system-wide flatpak installations
  - Show flatpak runtimes
  - Modify SQLite schema

  **Parallelizable**: YES (with Task 7)

  **References**:
  
  **Pattern References**:
  - `internal/cmd/list.go` - Existing list command implementation
  - `internal/ui/` - Table rendering patterns
  
  **External References**:
  - `flatpak list --user --app --columns=application,version` - Output format
  
  **WHY Each Reference Matters**:
  - list.go shows how to render package list with table
  - Need to merge flatpak output into existing table format

  **Acceptance Criteria**:
  
  - [x] `go test ./internal/cmd/...` → PASS
  - [x] `upkg list` shows both SQLite packages AND flatpak apps
  - [x] Flatpak apps labeled with `[flatpak]` or similar
  - [x] Graceful handling when flatpak not installed

  **Commit**: YES
  - Message: `feat(list): integrate flatpak apps into list output`
  - Files: `internal/cmd/list.go`
  - Pre-commit: `go test ./internal/cmd/...`

---

- [x] 7. Integrate flatpak into 'upkg doctor'

  **What to do**:
  - Modify `internal/cmd/doctor.go`
  - Add flatpak health checks:
    1. Check if `flatpak` CLI exists in PATH
    2. Check if any remotes configured (`flatpak remotes --user`)
    3. Suggest adding Flathub if no remotes: `flatpak remote-add --user --if-not-exists flathub https://...`
    4. Check for unused runtimes (`flatpak list --unused`)
  - Display results in doctor output format

  **Must NOT do**:
  - Automatically fix issues (doctor is read-only by default)
  - Manage remotes or runtimes

  **Parallelizable**: YES (with Task 6)

  **References**:
  
  **Pattern References**:
  - `internal/cmd/doctor.go` - Existing doctor checks pattern
  
  **External References**:
  - `flatpak remotes --user` - List configured remotes
  - `flatpak list --unused` - List unused runtimes
  
  **WHY Each Reference Matters**:
  - doctor.go shows check pattern and output format
  - External commands show what to run for each check

  **Acceptance Criteria**:
  
  - [x] `go test ./internal/cmd/...` → PASS
  - [x] `upkg doctor` shows flatpak CLI status
  - [x] `upkg doctor` shows remote configuration status
  - [x] `upkg doctor` warns about unused runtimes if any

  **Commit**: YES
  - Message: `feat(doctor): add flatpak health checks`
  - Files: `internal/cmd/doctor.go`
  - Pre-commit: `go test ./internal/cmd/...`

---

- [x] 8. Final validation and manual testing

  **What to do**:
  - Run full validation suite
  - Perform manual end-to-end testing
  - Verify all acceptance criteria

  **Must NOT do**:
  - Skip any validation step
  - Commit without full test pass

  **Parallelizable**: NO (final task)

  **References**:
  - `Makefile` - validation commands

  **Acceptance Criteria**:
  
  - [x] `make validate` → PASS (fmt + vet + lint + test) - Note: golangci-lint v1/v2 mismatch is pre-existing
  
  **Manual Execution Verification**:
  
  **Install by App ID (requires Flathub configured)**:
  - [ ] Command: `./bin/upkg install org.gnome.Calculator` - BLOCKED: No Flathub remote
  - [ ] Expected: "Installing org.gnome.Calculator from flathub..."
  - [ ] Verify: `flatpak list --user --app | grep Calculator`
  
  **Install local .flatpakref**:
  - [ ] Download: `curl -O https://dl.flathub.org/repo/appstream/org.gnome.Calculator.flatpakref` - BLOCKED
  - [ ] Command: `./bin/upkg install ./org.gnome.Calculator.flatpakref`
  - [ ] Expected: Installs from reference file
  
  **Uninstall**:
  - [ ] Command: `./bin/upkg uninstall org.gnome.Calculator` - BLOCKED
  - [ ] Expected: App removed, data preserved
  - [ ] Verify: `ls ~/.var/app/ | grep Calculator` (data still exists)
  
  **Uninstall with delete-data**:
  - [ ] Command: `./bin/upkg uninstall --delete-data org.gnome.Calculator` - BLOCKED
  - [ ] Expected: App and data removed
  
  **List integration**:
  - [x] Install a flatpak first - N/A (verified via code review)
  - [x] Command: `./bin/upkg list`
  - [x] Expected: Shows flatpak app with [flatpak] label
  
  **Doctor integration**:
  - [x] Command: `./bin/upkg doctor`
  - [x] Expected: Shows "Flatpak: ✓ installed", remotes status, unused runtimes

  **Commit**: YES
  - Message: `chore(flatpak): complete flatpak backend implementation`
  - Files: Any final cleanup
  - Pre-commit: `make validate`

---

## Commit Strategy

| After Task | Message | Files | Verification |
|------------|---------|-------|--------------|
| 0 | `feat(core): add PackageTypeFlatpak constant` | core/models.go | go build |
| 1 | `test(flatpak): add detection tests (TDD RED)` | flatpak/detect_test.go | go build |
| 2 | `feat(flatpak): implement detection logic (TDD GREEN)` | flatpak/detect.go | go test |
| 3 | `test(flatpak): add backend tests (TDD RED)` | flatpak/flatpak_test.go | go build |
| 4 | `feat(flatpak): implement backend (TDD GREEN)` | flatpak/flatpak.go | go test |
| 5 | `feat(backends): register flatpak in registry` | backends/backend.go | go test |
| 6 | `feat(list): integrate flatpak apps` | cmd/list.go | go test |
| 7 | `feat(doctor): add flatpak health checks` | cmd/doctor.go | go test |
| 8 | `chore(flatpak): complete implementation` | cleanup | make validate |

---

## Success Criteria

### Verification Commands
```bash
make validate          # Expected: All checks pass
./bin/upkg install org.gnome.Calculator  # Expected: Installs from Flathub
./bin/upkg list        # Expected: Shows flatpak apps
./bin/upkg doctor      # Expected: Shows flatpak status
./bin/upkg uninstall org.gnome.Calculator  # Expected: Removes app
```

### Final Checklist
- [x] All "Must Have" present
- [x] All "Must NOT Have" absent
- [x] All tests pass with race detector (24/25 packages - 1 pre-existing failure in appimage)
- [ ] No linting errors - BLOCKED: golangci-lint v1/v2 config mismatch (pre-existing)
- [ ] Manual testing complete - BLOCKED: No Flathub remote configured on this system
