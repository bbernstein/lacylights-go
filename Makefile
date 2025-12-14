# LacyLights Go Server Makefile

# Go binary
GO ?= go

# Binary name
BINARY_NAME := lacylights-go

# Build directory
BUILD_DIR := ./build

# Source directories
CMD_DIR := ./cmd/server

# Test directories
TEST_DIR := ./test

# Default target
.DEFAULT_GOAL := help

# Phony targets
.PHONY: all build clean test test-unit test-contracts test-coverage test-coverage-check generate dev run lint fmt help install-tools

# =============================================================================
# BUILD TARGETS
# =============================================================================

## all: Clean, generate, and build
all: clean generate build

## build: Build the server binary
build: generate
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GO) build -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_DIR)
	@echo "Built $(BUILD_DIR)/$(BINARY_NAME)"

## clean: Remove build artifacts
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR)
	@rm -f internal/graphql/generated/*.go
	@echo "Clean complete"

## generate: Generate GraphQL code
generate:
	@echo "Generating GraphQL code..."
	$(GO) run github.com/99designs/gqlgen generate
	@echo "Generation complete"

# =============================================================================
# RUN TARGETS
# =============================================================================

## run: Build and run the server
run: build
	@echo "Starting server..."
	$(BUILD_DIR)/$(BINARY_NAME)

## dev: Run with hot reload (requires Air)
dev:
	@echo "Starting development server with hot reload..."
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Air not installed. Install with: go install github.com/cosmtrek/air@latest"; \
		echo "Falling back to standard run..."; \
		$(MAKE) run; \
	fi

# =============================================================================
# TEST TARGETS
# =============================================================================

## test: Run all tests
test: test-unit

## test-unit: Run unit tests
test-unit:
	@echo "Running unit tests..."
	$(GO) test -v -race ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "Running tests with coverage..."
	$(GO) test -v -race -coverprofile=coverage.out ./...
	$(GO) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## test-coverage-check: Run tests and check coverage thresholds
test-coverage-check:
	@echo "Running coverage checks..."
	@./scripts/check-coverage.sh

## test-contracts: Run contract tests against Go server
test-contracts:
	@echo "Running contract tests against Go server..."
	$(GO) test -v ./test/contracts/...

## test-contracts-node: Run contract tests against Node server
test-contracts-node:
	@echo "Running contract tests against Node server..."
	GRAPHQL_ENDPOINT=http://localhost:4000/graphql $(GO) test -v ./test/contracts/...

## test-contracts-side-by-side: Run contract tests against both servers
test-contracts-side-by-side:
	@echo "=============================================="
	@echo "Running side-by-side contract tests"
	@echo "=============================================="
	@$(MAKE) test-contracts-compare

# =============================================================================
# LINT & FORMAT TARGETS
# =============================================================================

## lint: Run linters
lint:
	@echo "Running linters..."
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: brew install golangci-lint"; \
		$(GO) vet ./...; \
	fi

## fmt: Format code
fmt:
	@echo "Formatting code..."
	$(GO) fmt ./...
	@if command -v goimports > /dev/null; then \
		goimports -w .; \
	fi

# =============================================================================
# TOOL INSTALLATION
# =============================================================================

## install-tools: Install development tools
install-tools:
	@echo "Installing development tools..."
	$(GO) install github.com/99designs/gqlgen@latest
	$(GO) install github.com/cosmtrek/air@latest
	$(GO) install golang.org/x/tools/cmd/goimports@latest
	@echo "Tools installed"

# =============================================================================
# SCHEMA TARGETS
# =============================================================================

## schema-export: Export GraphQL schema for CI/CD validation
schema-export:
	@echo "Exporting GraphQL schema..."
	@cp internal/graphql/schema/schema.graphql schema.graphql
	@echo "Schema exported to schema.graphql"

## schema-diff: Compare schema with test contracts
schema-diff:
	@echo "Comparing GraphQL schemas with test contracts..."
	@diff -u internal/graphql/schema/schema.graphql ../lacylights-test/contracts/schema.graphql 2>/dev/null || echo "No test contract schema found"

# =============================================================================
# DATABASE TARGETS
# =============================================================================

## db-path: Show database path
db-path:
	@echo "Database path: $${DATABASE_URL:-file:./dev.db}"

# =============================================================================
# HELP
# =============================================================================

## help: Show this help message
help:
	@echo "LacyLights Go Server"
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed -e 's/^## /  /'
