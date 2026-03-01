# Hourglass Rejeições RPA Makefile
.PHONY: all build test clean lint fmt vet coverage docker-build docker-run help

# Variables
BINARY_NAME=rpa
BUILD_DIR=./build
DOCKER_IMAGE=hourglass-rejeicoes-rpa
GO=go
GOFLAGS=-v

# Default target
all: clean lint test build

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## //'

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/rpa

## test: Run all tests
test:
	@echo "Running tests..."
	$(GO) test $(GOFLAGS) -race -coverprofile=coverage.out ./...

## test-short: Run tests without race detector (faster)
test-short:
	@echo "Running tests (short)..."
	$(GO) test $(GOFLAGS) ./...

## coverage: Generate and display test coverage
coverage: test
	@echo "Generating coverage report..."
	$(GO) tool cover -html=coverage.out -o coverage.html
	$(GO) tool cover -func=coverage.out

## coverage-total: Show total coverage percentage
coverage-total: test
	@$(GO) tool cover -func=coverage.out | grep total | awk '{print "Total coverage: " $$3}'

## lint: Run golangci-lint
lint:
	@echo "Running linter..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install from https://golangci-lint.run/usage/install/"; \
		exit 1; \
	fi

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GO) vet ./...

## tidy: Tidy and verify Go modules
tidy:
	@echo "Tidying modules..."
	$(GO) mod tidy
	$(GO) mod verify

## clean: Clean build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f coverage.out coverage.html

## run: Run the application (requires .env)
run: build
	@echo "Running $(BINARY_NAME)..."
	./$(BUILD_DIR)/$(BINARY_NAME)

## run-once: Run once mode
run-once: build
	@echo "Running once mode..."
	./$(BUILD_DIR)/$(BINARY_NAME) -once

## run-setup: Run setup mode
run-setup: build
	@echo "Running setup mode..."
	./$(BUILD_DIR)/$(BINARY_NAME) -setup

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build -t $(DOCKER_IMAGE):latest .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it --env-file .env $(DOCKER_IMAGE):latest

## docker-compose-up: Start with Docker Compose
docker-compose-up:
	@echo "Starting with Docker Compose..."
	docker-compose up -d

## docker-compose-down: Stop Docker Compose
docker-compose-down:
	@echo "Stopping Docker Compose..."
	docker-compose down

## vulncheck: Run govulncheck
vulncheck:
	@echo "Running vulnerability check..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
		exit 1; \
	fi

## ci: Run all CI checks
ci: tidy fmt vet lint test coverage-total vulncheck

## install-tools: Install required development tools
install-tools:
	@echo "Installing development tools..."
	$(GO) install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

.DEFAULT_GOAL := help
