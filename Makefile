.PHONY: build test lint install clean fmt vet coverage help

# Build variables
BINARY_NAME=upkg
BUILD_DIR=bin
CMD_DIR=cmd/upkg
VERSION?=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"

# Go commands
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOVET=$(GOCMD) vet
GOFMT=gofmt
GOMOD=$(GOCMD) mod
GOLINT=golangci-lint

## help: Show this help message
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' | sed -e 's/^/ /'

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

## test-coverage: Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	$(GOLINT) run ./...

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	$(GOFMT) -s -w .

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

## tidy: Tidy go modules
tidy:
	@echo "Tidying go modules..."
	$(GOMOD) tidy

## install: Install the binary to GOBIN or GOPATH/bin
install: build
	@echo "Installing $(BINARY_NAME)..."
	@if [ -z "$(shell go env GOBIN)" ]; then \
		mkdir -p $(shell go env GOPATH)/bin; \
		cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/$(BINARY_NAME); \
		echo "Installed to $(shell go env GOPATH)/bin/$(BINARY_NAME)"; \
	else \
		cp $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOBIN)/$(BINARY_NAME); \
		echo "Installed to $(shell go env GOBIN)/$(BINARY_NAME)"; \
	fi

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html
	@echo "Clean complete"

## validate: Run all validation checks (fmt, vet, lint, test)
validate: fmt vet lint test
	@echo "All validation checks passed!"

## quick-check: Quick validation (fmt + vet + lint)
quick-check: fmt vet lint
	@echo "Quick checks passed!"

## coverage: Generate and display coverage
coverage: test-coverage
	@echo "Opening coverage report..."
	@$(GOCMD) tool cover -func=coverage.out

## run: Build and run the application
run: build
	@./$(BUILD_DIR)/$(BINARY_NAME)

.DEFAULT_GOAL := help
