# Refactoring Plan: Enhanced `upkg uninstall` Command

## 1. Executive Summary

This document tracks the evolution of the `upkg uninstall` command interface. The initial refactoring goals have been **completed**, and this plan now focuses on **Phase 2 enhancements** to further improve usability, scripting support, and UX.

---

## 2. Completed Features (Phase 1)

The following features from the original plan have been implemented:

| Feature | Status | Location |
|---------|--------|----------|
| Interactive package selection | ✅ Done | `uninstall.go:64-188` |
| Fuzzy search filtering | ✅ Done | `prompt.go:116-125` (via `fuzzysearch`) |
| Multi-select (bulk uninstall) | ✅ Done | `prompt.go:101-162` |
| Confirmation dialog | ✅ Done | `uninstall.go:141-150` |
| Disk space calculation | ✅ Done | `uninstall.go:119-138` |
| Progress feedback (`[1/N]`) | ✅ Done | `uninstall.go:159-175` |
| Color-coded output | ✅ Done | Using `fatih/color` |

---

## 3. Phase 2: Implemented Improvements

### 3.1. Bulk Uninstall via CLI Arguments (DONE)

**Priority:** High
**Status:** Implemented
**Rationale:** Users should be able to uninstall multiple packages in a single command without interactive mode.

**Current behavior:**
```bash
upkg uninstall package1  # Single package only
upkg uninstall           # Interactive multi-select
```

**Proposed behavior:**
```bash
upkg uninstall package1 package2 package3  # Bulk uninstall via args
```

**Implementation:**
1. Change `Args: cobra.MaximumNArgs(1)` → `Args: cobra.ArbitraryArgs`
2. Add `runBulkUninstall()` function to handle multiple arguments
3. Iterate over args, collect valid packages, show summary, confirm, execute

**Files to modify:**
- `internal/cmd/uninstall.go`

---

### 3.2. Non-Interactive Mode (`--yes` / `-y`) (DONE)

**Priority:** High
**Status:** Implemented
**Rationale:** Enable usage in scripts, CI/CD pipelines, and automation without TTY.

**Proposed flags:**
```bash
upkg uninstall package1 --yes        # Skip confirmation
upkg uninstall package1 -y           # Short form
upkg uninstall --all --yes           # Uninstall all (dangerous, requires --yes)
```

**Implementation:**
1. Add `--yes` / `-y` flag to skip confirmation prompts
2. Detect non-TTY environment and require `--yes` flag:
   ```go
   if !term.IsTerminal(int(os.Stdin.Fd())) && !yesFlag {
       return fmt.Errorf("non-interactive mode requires --yes flag")
   }
   ```
3. When `--yes` is set, skip `ui.ConfirmPrompt()` calls

**Dependencies:**
- `golang.org/x/term` (for TTY detection)

**Files to modify:**
- `internal/cmd/uninstall.go`

---

### 3.3. Dry-Run Mode (`--dry-run`) (DONE)

**Priority:** Medium
**Status:** Implemented
**Rationale:** Allow users to preview what would be removed without executing.

**Proposed behavior:**
```bash
upkg uninstall package1 --dry-run
# Output:
# [DRY-RUN] Would uninstall:
#   • package1 (AppImage) - 150.2 MB
#   • Files: ~/.local/share/upkg/apps/package1/
#   • Desktop: ~/.local/share/applications/package1.desktop
#   • Icons: 3 files in ~/.local/share/icons/
# No changes made.
```

**Implementation:**
1. Add `--dry-run` flag
2. In `performUninstall()`, check flag and skip actual deletion
3. Display detailed breakdown of files that would be removed

**Files to modify:**
- `internal/cmd/uninstall.go`

---

### 3.4. Improved Multi-Select UX (DONE)

**Priority:** Medium
**Status:** Implemented using `survey/v2`
**Rationale:** The current multi-select implementation uses sequential selection (pick → remove from list → repeat), which is non-standard.

**Current flow:**
```
Select packages to uninstall (select multiple, choose 'Done' when finished)
▸ package1
  package2
  [Done - Finish selection]
# User selects package1 → it disappears → repeat
```

**Proposed flow (checkbox-style):**
```
Select packages to uninstall (Space to toggle, Enter to confirm)
▸ [x] package1
  [ ] package2
  [ ] package3
```

**Implementation options:**
1. **Option A:** Migrate to `charmbracelet/bubbletea` + `charmbracelet/bubbles` (full TUI framework)
2. **Option B:** Migrate to `AlecAivazis/survey/v2` (simpler, has native MultiSelect)
3. **Option C:** Keep `promptui` but implement custom checkbox renderer

**Recommendation:** Option B (`survey/v2`) - minimal migration effort, native checkbox support, actively maintained.

**Files to modify:**
- `internal/ui/prompt.go`
- `go.mod` (add new dependency)

---

### 3.5. Uninstall All (`--all`) (DONE)

**Priority:** Low
**Status:** Implemented
**Rationale:** Convenience for system reset or cleanup scenarios.

**Proposed behavior:**
```bash
upkg uninstall --all --yes  # Uninstall everything
upkg uninstall --all        # Prompts for confirmation with full package list
```

**Safety:**
- Require explicit `--yes` flag when used non-interactively
- Show detailed summary before confirmation
- Log all removals for audit

**Files to modify:**
- `internal/cmd/uninstall.go`

---

### 3.6. Parallel Uninstallation

**Priority:** Low
**Rationale:** When uninstalling many packages, parallel execution could speed up the process.

**Considerations:**
- Database writes must remain serialized (SQLite limitation)
- File operations can be parallelized
- Need to handle partial failures gracefully
- Consider `--parallel` flag with worker count (default: `runtime.NumCPU()`)

**Deferred:** Complexity vs. benefit ratio is low for typical use cases (< 10 packages).

---

## 4. Technical Considerations

### 4.1. Error Handling Strategy

For bulk operations, adopt a **continue-on-error** strategy:
```go
type UninstallResult struct {
    Name    string
    Success bool
    Error   error
}

// Collect all results, report summary at end
```

### 4.2. Logging

All uninstall operations should be logged with:
- Package name and ID
- Timestamp
- Success/failure status
- Files removed (at DEBUG level)

### 4.3. Backwards Compatibility

- All new flags are optional
- Default behavior remains unchanged
- Existing scripts using `upkg uninstall <name>` continue to work

---

## 5. Implementation Status

| Phase | Task | Status |
|-------|------|--------|
| 2.1 | Bulk uninstall via CLI args | ✅ Done |
| 2.2 | `--yes` / `-y` flag | ✅ Done |
| 2.3 | `--dry-run` flag | ✅ Done |
| 2.4 | Improved multi-select UX | ✅ Done |
| 2.5 | `--all` flag | ✅ Done |
| 2.6 | Parallel uninstallation | Deferred |

**Implementation completed:** 2.1 → 2.2 → 2.3 → 2.5 → 2.4

---

## 6. CLI Reference (Post-Implementation)

```bash
# Single package
upkg uninstall <package-name>
upkg uninstall <install-id>

# Multiple packages (new)
upkg uninstall pkg1 pkg2 pkg3

# Interactive mode
upkg uninstall

# Flags
upkg uninstall <pkg> --yes          # Skip confirmation
upkg uninstall <pkg> -y             # Short form
upkg uninstall <pkg> --dry-run      # Preview only
upkg uninstall --all --yes          # Remove all packages
upkg uninstall --timeout 300        # Custom timeout (seconds)
```

---

## 7. Success Criteria

- [x] `upkg uninstall pkg1 pkg2` works correctly
- [x] `upkg uninstall pkg1 --yes` skips confirmation
- [x] `upkg uninstall pkg1 --dry-run` shows preview without deletion
- [x] Non-TTY environments fail gracefully with helpful error message
- [x] All new features have 80%+ test coverage (achieved: 80.83%)
- [ ] `make validate` passes (blocked by pre-existing lint issues)

---

## 8. References

- Original implementation: `internal/cmd/uninstall.go`
- UI utilities: `internal/ui/prompt.go`
- Similar patterns: `internal/cmd/install.go` (for flag conventions)
