# Code Style & Conventions

## Formatting (from .editorconfig)
- **Indentation**: Tabs (size 4)
- **Max line length**: 120 characters
- **Charset**: utf-8
- **Trailing whitespace**: Trimmed
- **Final newline**: Required

## Import Organization
Group imports as: Stdlib -> 3rd Party -> Local
```go
import (
    "context"
    "fmt"
    
    "github.com/spf13/cobra"
    "github.com/rs/zerolog"
    
    "upkg/internal/config"
    "upkg/internal/logging"
)
```

## Naming Conventions
- **Interfaces**: Define in `internal/core/` and `internal/backends/`
- **Backend implementations**: Separate packages under `internal/backends/<format>/`
- **Context parameter**: Always `ctx context.Context` as first parameter for I/O operations
- **Error handling**: Always check errors, use `zerolog` for logging

## Logging
- Use `internal/logging` package
- NEVER use `fmt.Printf` for logs
- Structured logging with zerolog

## Error Handling
- Never ignore errors
- Always wrap with context: `fmt.Errorf("action: %w", err)`
- Log context with errors (file paths, package names, etc.)
- Return errors up the stack, handle at appropriate level

## Security
- Validate paths/input via `internal/security`
- Path validation before filesystem operations
- Prevent directory traversal attacks

## Testing Conventions
- Test files: `*_test.go`
- Use `afero` filesystem mocking for all filesystem operations
- Race detector enabled: `go test -race`
- Table-driven tests preferred
- Co-locate tests with source files

## Linting Configuration (from .golangci.yml)

### Enabled Linters
- **errcheck** - Check unchecked errors
- **gosimple** - Simplify code
- **govet** - Suspicious constructs
- **staticcheck** - Comprehensive static analysis
- **unused** - Unused code
- **gosec** - Security issues
- **gofmt** - Formatting
- **goimports** - Import ordering
- **misspell** - Spelling
- **goconst** - Repeated strings → constants
- **gocyclo** - Cyclomatic complexity (max: 15)
- **revive** - Extensible linter

### Security Settings
- **G204 excluded**: Subprocess with variable allowed (needed for package installation)

### Test File Relaxations
Excluded for `*_test.go`:
- gocyclo (complexity)
- errcheck (unchecked errors)
- gosec (security)
- unparam (unused parameters)

## File Organization
- Package declaration
- Imports (grouped as above)
- Constants
- Types/Interfaces
- Functions
- Methods (grouped by receiver)

## Comments
- Package comments: Above package declaration
- Exported symbols: Comment starting with symbol name
- Complex logic: Inline comments explaining "why", not "what"
