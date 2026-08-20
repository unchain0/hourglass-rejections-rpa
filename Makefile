# Hourglass Rejections RPA Makefile
.PHONY: all build build-rpa build-save-tokens build-token-refresh build-setup-auth test clean lint fmt vet coverage coverage-check docker-build docker-run docker-compose-up docker-compose-down docker-auth-bootstrap help run run-once save-tokens token-refresh setup-auth copy-to-vps copy-to-vps-password

# Variables
BINARY_NAME=rpa
SAVE_TOKENS_NAME=save-tokens
TOKEN_REFRESH_NAME=token-refresh
SETUP_AUTH_NAME=setup-auth
BUILD_DIR=.
DOCKER_IMAGE=hourglass-rejections-rpa
APP_VERSION?=dev
GIT_COMMIT?=$(shell git rev-parse --short=12 HEAD 2>/dev/null || echo unknown)
GO=go
GOFLAGS=-v
# Ensure Go downloads the correct toolchain version from go.mod
GOTOOLCHAIN?=auto
export GOTOOLCHAIN

# Default target
all: clean lint test build

## help: Show this help message
help:
	@echo "Available targets:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## //'

## build: Build all binaries
build: build-rpa build-save-tokens build-token-refresh build-setup-auth

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

## build-setup-auth: Build the setup-auth utility
build-setup-auth:
	@echo "Building $(SETUP_AUTH_NAME)..."
	$(GO) build $(GOFLAGS) -o $(BUILD_DIR)/$(SETUP_AUTH_NAME) ./cmd/setup-auth

## test: Run all tests
test:
	@echo "Running tests..."
	$(GO) test $(GOFLAGS) -race -coverprofile=coverage.out ./...
	$(MAKE) coverage-check

## coverage-check: Require 100% statement coverage in every package
coverage-check:
	@bash scripts/check-coverage.sh coverage.out

## test-short: Run tests without race detector (faster)
test-short:
	@echo "Running tests (short)..."
	$(GO) test $(GOFLAGS) -short -timeout 5m ./...

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
		golangci-lint run --timeout=5m ./...; \
	else \
		echo "golangci-lint not installed. Run: make install-tools"; \
		exit 1; \
	fi

## fmt: Format Go code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...

## fmt-check: Check Go code formatting (CI-safe, does not modify files)
fmt-check:
	@echo "Checking code formatting..."
	@UNFORMATTED=$$($(GO) fmt ./...); \
	if [ -n "$$UNFORMATTED" ]; then \
		echo "ERROR: The following files are not formatted:"; \
		echo "$$UNFORMATTED"; \
		echo "Run 'make fmt' to fix"; \
		exit 1; \
	fi; \
	echo "All Go files are properly formatted"

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
	@rm -f $(BINARY_NAME) $(SAVE_TOKENS_NAME) $(TOKEN_REFRESH_NAME) $(SETUP_AUTH_NAME) register-webauthn
	@rm -f coverage.out coverage.html

## run: Run the application (requires .env)
run: build-rpa
	@echo "Running $(BINARY_NAME)..."
	./$(BINARY_NAME)

## run-once: Run once mode
run-once: build-rpa
	@echo "Running once mode..."
	./$(BINARY_NAME) -once

## save-tokens: Authenticate and save tokens for immediate/manual VPS use
save-tokens: build-save-tokens
	@echo "Running save-tokens..."
	./$(SAVE_TOKENS_NAME)

## token-refresh: Try to refresh tokens automatically
## Usage: make token-refresh
token-refresh: build-token-refresh
	@echo "Running token-refresh..."
	./$(TOKEN_REFRESH_NAME)

## setup-auth: Provision tokens + WebAuthn credentials for automatic renewal
setup-auth: build-setup-auth
	@echo "Running setup-auth..."
	./$(SETUP_AUTH_NAME)

## copy-to-vps: Copy tokens and WebAuthn credentials to a VPS using SSH keys
## Usage: make copy-to-vps VPS=user@your-vps.com
copy-to-vps:
	@if [ -z "$(VPS)" ]; then \
		echo "Usage: make copy-to-vps VPS=user@your-vps.com"; \
		echo ""; \
		echo "If your VPS uses password authentication, use: make copy-to-vps-password"; \
		exit 1; \
	fi
	@echo "Copying authentication files to $(VPS)..."
	@ssh "$(VPS)" 'install -d -m 700 ~/.hourglass-rpa' && \
		scp ~/.hourglass-rpa/auth-tokens.json ~/.hourglass-rpa/webauthn-credentials.json "$(VPS):~/.hourglass-rpa/" || \
		(echo "❌ copy failed!" && echo "For password auth, use: make copy-to-vps-password VPS=$(VPS)" && exit 1)

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
		echo "2. METHOD: use this target with the VPS address"; \
		echo "   make copy-to-vps-password VPS=user@your-vps.com"; \
		echo ""; \
		exit 1; \
	fi
	@echo "=== Copying to $(VPS) with password auth ==="
	@echo "You will be prompted for your VPS password..."
	@echo ""
	@ssh "$(VPS)" 'install -d -m 700 ~/.hourglass-rpa'
	@scp ~/.hourglass-rpa/auth-tokens.json ~/.hourglass-rpa/webauthn-credentials.json "$(VPS):~/.hourglass-rpa/"

## docker-build: Build Docker image
docker-build:
	@echo "Building Docker image..."
	docker build --pull --build-arg APP_VERSION=$(APP_VERSION) --build-arg VCS_REF=$(GIT_COMMIT) -t $(DOCKER_IMAGE):$(APP_VERSION) .

## docker-run: Run Docker container
docker-run:
	@echo "Running Docker container..."
	docker run --rm -it --env-file .env $(DOCKER_IMAGE):$(APP_VERSION)

## docker-compose-up: Start with Docker Compose
docker-compose-up:
	@echo "Starting with Docker Compose..."
	APP_VERSION=$(APP_VERSION) GIT_COMMIT=$(GIT_COMMIT) docker compose up -d --build

## docker-compose-down: Stop Docker Compose
docker-compose-down:
	@echo "Stopping Docker Compose..."
	docker compose down

## docker-auth-bootstrap: Import locally generated authentication into the Compose volume
## Usage: make docker-auth-bootstrap AUTH_SOURCE_DIR=$$HOME/.hourglass-rpa
docker-auth-bootstrap:
	@test -n "$(AUTH_SOURCE_DIR)" || (echo "AUTH_SOURCE_DIR is required" && exit 1)
	AUTH_SOURCE_DIR="$(AUTH_SOURCE_DIR)" docker compose --profile bootstrap run --rm auth-bootstrap

## vulncheck: Run govulncheck
vulncheck:
	@echo "Running vulnerability check..."
	@if command -v govulncheck >/dev/null 2>&1; then \
		govulncheck ./...; \
	else \
		echo "govulncheck not installed. Run: go install golang.org/x/vuln/cmd/govulncheck@latest"; \
		exit 1; \
	fi

## ci: Run all CI checks (CI server order)
ci: tidy fmt vet lint test coverage-total vulncheck

## ci-local: Run all checks exactly like CI but optimized for local use (excludes slow webauthn tests)
ci-local: tidy fmt-check vet lint test-short vulncheck build
	@echo "✅ All local CI checks passed"

## install-tools: Install required development tools (pinned to CI versions)
install-tools:
	@echo "Installing development tools..."
	$(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.1.6
	$(GO) install golang.org/x/vuln/cmd/govulncheck@latest

.DEFAULT_GOAL := help
