# Flatpak Backend - Learnings

## 2026-01-19 Implementation Complete

### Successful Patterns

1. **Delegate Backend Pattern**: Flatpak CLI manages everything (install, uninstall, desktop, icons). No SQLite tracking needed - just wrap CLI.

2. **Detection Priority**: Flatpak must be FIRST in registry to capture App ID patterns (e.g., `org.gnome.Calculator`) before file-based backends try to process them.

3. **App ID Regex**: `^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*){2,}$` - requires at least 3 components (org.domain.App).

4. **File Detection**:
   - `.flatpak`: Check for ZIP magic (`PK\x03\x04`) for OCI bundles
   - `.flatpakref`: Check for `[Flatpak Ref]` INI header

5. **CLI Flags**: Always use `--user --noninteractive` for consistent behavior.

### Conventions Discovered

- Backend constructors: `New()`, `NewWithDeps()`, `NewWithRunner()` pattern
- Error wrapping: Always use `fmt.Errorf("context: %w", err)`
- Mock testing: Use `helpers.NewMockCommandRunner()` with `AddResponse()`

### Technical Notes

- Flatpak handles its own rollback (OSTree atomic operations)
- No need for `tx.Add()` rollback registration
- Desktop integration automatic via flatpak exports
- User scope only (`~/.local/share/flatpak/`)
