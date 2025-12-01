# Suggested Commands for upkg

## Build & Run
```bash
make build              # Build binary to bin/upkg
make install            # Install to $GOBIN or $GOPATH/bin
./bin/upkg --help       # Run built binary directly
```

## Testing
```bash
make test               # Run all tests with race detector
make test-coverage      # Generate coverage report (coverage.html)
```

## Code Quality & Validation
```bash
make fmt                # Format code with gofmt
make lint               # Run golangci-lint
make validate           # Run fmt + vet + lint + test (FULL VALIDATION)
make quick-check        # Run fmt + vet + lint (skip tests)
```

## Task Completion Workflow

**After ANY code modification:**
```bash
make validate
```

This ensures:
1. Code is formatted (`gofmt`)
2. No suspicious constructs (`go vet`)
3. All linters pass (`golangci-lint`)
4. All tests pass with race detector

## Running the Application

```bash
# After building
./bin/upkg install <package>
./bin/upkg list
./bin/upkg info <name>
./bin/upkg uninstall <name>
./bin/upkg doctor
```

## Running a Single Test
```bash
go test -v -race -run TestName ./path/to/pkg
```
