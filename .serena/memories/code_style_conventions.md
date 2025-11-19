# Code Style & Conventions

## Formatting (from .editorconfig)
- **Indentation**: Tabs (size 4)
- **Max line length**: 120 characters
- **Charset**: utf-8
- **Trailing whitespace**: Trimmed
- **Final newline**: Required

## Naming Conventions
- **Interfaces**: Define in `internal/core/` and `internal/backends/`
- **Backend implementations**: Separate packages under `internal/backends/<format>/`
- **Context parameter**: Always `ctx context.Context` as first parameter for I/O operations
- **Error handling**: Always check errors, log with `zerolog`

## Go Conventions
- Use `gofmt` for formatting
- Follow standard Go project layout
- Package names: lowercase, single word
- Exported names: Start with uppercase
- Unexported names: Start with lowercase

## Error Handling
- Never ignore errors
- Use `zerolog` for structured logging
- Log context with errors (file paths, package names, etc.)
- Return errors up the stack, handle at appropriate level

## Testing Conventions
- Test files: `*_test.go`
- Use `afero` filesystem mocking for all filesystem operations
- Race detector enabled: `go test -race`
- Table-driven tests preferred
- Coverage: Generate HTML reports with `make test-coverage`

## Linting Configuration (from .golangci.yml)

### Enabled Linters
- **errcheck** - Check unchecked errors
- **gosimple** - Simplify code
- **govet** - Suspicious constructs
- **ineffassign** - Ineffectual assignments
- **staticcheck** - Comprehensive static analysis
- **unused** - Unused code
- **gosec** - Security issues
- **gofmt** - Formatting
- **goimports** - Import ordering
- **misspell** - Spelling
- **unparam** - Unused function parameters
- **unconvert** - Unnecessary type conversions
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

## Import Organization
```go
// Standard library
import (
    "context"
    "fmt"
)

// External dependencies
import (
    "github.com/spf13/cobra"
    "github.com/rs/zerolog"
)

// Internal packages
import (
    "github.com/diogo/upkg/internal/config"
    "github.com/diogo/upkg/internal/logging"
)
```

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
