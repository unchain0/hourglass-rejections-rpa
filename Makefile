# Hourglass Rejections RPA Makefile
.PHONY: all build build-rpa build-save-tokens build-token-refresh test clean lint fmt vet coverage docker-build docker-run help run run-once save-tokens token-refresh copy-to-vps copy-to-vps-password

# Variables
BINARY_NAME=rpa
SAVE_TOKENS_NAME=save-tokens
TOKEN_REFRESH_NAME=token-refresh
BUILD_DIR=.
DOCKER_IMAGE=hourglass-rejections-rpa
GO=go
GOFLAGS=-v

# Default target
all: clean lint test build

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## //'

## build: Build all binaries
build: build-rpa build-save-tokens build-token-refresh

## build-rpa: Build the main rpa binary
build-rpa:
	@echo "Building $(BINARY_NAME)..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/rpa

## build-save-tokens: Build the save-tokens utility
build-save-tokens:
	@echo "Building $(SAVE_TOKENS_NAME)..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(SAVE_TOKENS_NAME) ./cmd/save-tokens

## build-token-refresh: Build the token-refresh utility
build-token-refresh:
	@echo "Building $(TOKEN_REFRESH_NAME)..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(TOKEN_REFRESH_NAME) ./cmd/token-refresh

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
	@rm -f $(BINARY_NAME) $(SAVE_TOKENS_NAME) $(TOKEN_REFRESH_NAME) register-webauthn
	@rm -f coverage.out coverage.html

## run: Run the application (requires .env)
run: build-rpa
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)

## run-once: Run once mode
run-once: build-rpa
	@echo "Running once mode..."
	./$(BINARY_NAME) -once

## save-tokens: Authenticate and save tokens for VPS
save-tokens: build-save-tokens
	@echo "Running save-tokens..."
	./$(SAVE_TOKENS_NAME)

## token-refresh: Try to refresh tokens automatically
## Usage: make token-refresh
token-refresh: build-token-refresh
	@echo "Running token-refresh..."
	./$(TOKEN_REFRESH_NAME)

## copy-to-vps: Copy saved tokens to VPS using SSH keys
## Usage: make copy-to-vps VPS=user@your-vps.com
copy-to-vps:
	@if [ -z "$(VPS)" ]; then \
		echo "Usage: make copy-to-vps VPS=user@your-vps.com"; \
		echo ""; \
		echo "If your VPS uses password authentication, use: make copy-to-vps-password"; \
		exit 1; \
	fi
	@echo "Copying tokens to $(VPS)..."
	@scp ~/.hourglass-rpa/auth-tokens.json $(VPS):~/.hourglass-rpa/ 2>/dev/null || (echo "❌ scp failed!" && echo "For password auth, use: make copy-to-vps-password VPS=$(VPS)")

## copy-to-vps-password: Copy tokens to VPS with password (interactive)
## Usage: make copy-to-vps-password VPS=user@your-vps.com
copy-to-vps-password:
	@if [ -z "$(VPS)" ]; then \
		echo "=== VPS with Password Authentication ==="; \
		echo ""; \
		echo "Usage: make copy-to-vps-password VPS=user@your-vps.com"; \
		echo ""; \
		echo "Options for VPS with password auth:"; \
		echo ""; \
		echo "1. METHOD: SSH key setup (RECOMMENDED - do once)"; \
		echo "   ssh-copy-id user@your-vps.com"; \
		echo "   # Then use: make copy-to-vps VPS=user@your-vps.com"; \
		echo ""; \
		echo "2. METHOD: scp with password prompt"; \
		echo "   scp ~/.hourglass-rpa/auth-tokens.json user@your-vps.com:~/.hourglass-rpa/"; \
		echo ""; \
		echo "3. METHOD: Manual copy (copy-paste)"; \
		echo "   a. Show tokens: cat ~/.hourglass-rpa/auth-tokens.json"; \
		echo "   b. Copy the output"; \
		echo "   c. On VPS: mkdir -p ~/.hourglass-rpa"; \
		echo "   d. On VPS: nano ~/.hourglass-rpa/auth-tokens.json"; \
		echo "   e. Paste and save"; \
		echo ""; \
		exit 1; \
	fi
	@echo "=== Copying to $(VPS) with password auth ==="
	@echo "You will be prompted for your VPS password..."
	@echo ""
	@scp ~/.hourglass-rpa/auth-tokens.json $(VPS):~/.hourglass-rpa/

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
