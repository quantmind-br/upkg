# Task Completion Checklist

## After ANY Code Modification

Run the full validation suite:
```bash
make validate
```

This executes:
1. `make fmt` - Format code with gofmt
2. `make vet` - Run go vet
3. `make lint` - Run golangci-lint
4. `make test` - Run all tests with race detector

**All steps must pass before considering task complete.**

## Quick Check (Skip Tests)

For rapid feedback during development:
```bash
make quick-check
```

This runs: fmt + vet + lint (skips tests)

## Individual Commands

If you need to run specific checks:
```bash
make fmt        # Format only
make lint       # Lint only
make test       # Test only
```

## Coverage Analysis

After adding tests:
```bash
make test-coverage    # Generate coverage.html
```

## Pre-Commit Workflow

1. Make code changes
2. Run `make validate`
3. Fix any issues
4. Commit when all checks pass

## CI/CD Expectations

The following must always pass:
- All tests with race detector
- Zero linting errors
- Code formatted with gofmt
- Go vet reports no issues
- All dependencies properly declared

## Common Issues

**Tests failing:**
- Check race conditions (race detector enabled)
- Verify afero filesystem mocks
- Check test isolation

**Lint errors:**
- Run `make fmt` first
- Check cyclomatic complexity (max: 15)
- Review gosec security warnings
- Ensure proper error handling

**Build errors:**
- Run `go mod tidy` to sync dependencies
- Check import paths
- Verify Go version (1.25.3)
