.PHONY: build test test-coverage clean install fmt vet lint

# Build variables
BINARY_NAME=manager
BUILD_DIR=bin
CMD_DIR=cmd/manager
VERSION=1.0.0

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt

# Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -o $(BUILD_DIR)/$(BINARY_NAME) -ldflags="-s -w" ./$(CMD_DIR)
	@echo "Build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Build for production with optimizations
build-prod:
	@echo "Building $(BINARY_NAME) for production..."
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) \
		-o $(BUILD_DIR)/$(BINARY_NAME) \
		-ldflags="-s -w -X main.Version=$(VERSION)" \
		-trimpath \
		./$(CMD_DIR)
	@echo "Production build complete: $(BUILD_DIR)/$(BINARY_NAME)"

# Run tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race -timeout 120s ./...

# Run tests with coverage
test-coverage:
	@echo "Running tests with coverage..."
	$(GOTEST) -v -race -coverprofile=coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Run benchmarks
bench:
	@echo "Running benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...

# Clean build artifacts
clean:
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	@echo "Clean complete"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy

# Format code
fmt:
	@echo "Formatting code..."
	$(GOFMT) ./...

# Run go vet
vet:
	@echo "Running go vet..."
	$(GOCMD) vet ./...

# Install the binary
install: build
	@echo "Installing $(BINARY_NAME)..."
	cp $(BUILD_DIR)/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Install complete"

# Run all checks (fmt, vet, test)
check: fmt vet test
	@echo "All checks passed"

# Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t flush-manager:$(VERSION) .
	docker tag flush-manager:$(VERSION) flush-manager:latest

# Display help
help:
	@echo "Flush Manager - Makefile commands:"
	@echo ""
	@echo "  make build          - Build the binary"
	@echo "  make build-prod     - Build optimized binary for production"
	@echo "  make test           - Run tests"
	@echo "  make test-coverage  - Run tests with coverage report"
	@echo "  make bench          - Run benchmarks"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make deps           - Install dependencies"
	@echo "  make fmt            - Format code"
	@echo "  make vet            - Run go vet"
	@echo "  make install        - Install binary to /usr/local/bin"
	@echo "  make check          - Run fmt, vet, and test"
	@echo "  make docker-build   - Build Docker image"
	@echo "  make help           - Display this help message"

# Default target
.DEFAULT_GOAL := build
