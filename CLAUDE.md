# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

LacyLights Go is a high-performance Go implementation of the LacyLights lighting control system backend. It provides a GraphQL API for managing theatrical lighting fixtures, scenes, cue lists, and real-time DMX output via Art-Net.

## Development Commands

### Building
- `make build` - Build the server binary
- `make build-all` - Build for all platforms (linux/darwin, amd64/arm64)
- `go build ./cmd/server` - Direct Go build

### Testing
- `make test` - Run all tests
- `make test-coverage` - Run tests with coverage report
- `go test -v ./...` - Run tests directly
- `ARTNET_ENABLED=false go test ./...` - Run tests without Art-Net

### Linting
- `make lint` - Run golangci-lint
- `golangci-lint run` - Direct lint command

### Code Generation
- `make generate` - Generate GraphQL resolver code
- `go generate ./...` - Direct generate command

## Architecture

### Package Structure

- `cmd/server/` - Main application entry point
- `internal/config/` - Configuration management via environment variables
- `internal/database/` - SQLite database layer with Prisma-style migrations
- `internal/graph/` - GraphQL schema, resolvers, and generated code
- `internal/models/` - Domain models and business logic
- `internal/artnet/` - Art-Net protocol implementation for DMX output
- `pkg/` - Public packages for external use
- `test/` - Integration and end-to-end tests

### Key Technologies

- **GraphQL**: gqlgen for type-safe GraphQL
- **Database**: SQLite with custom ORM layer
- **Art-Net**: UDP-based DMX protocol (port 6454)
- **WebSocket**: For real-time subscriptions

## Important Patterns

### GraphQL Resolvers
All resolvers are in `internal/graph/`. After modifying `schema.graphqls`, run `make generate` to update resolver stubs.

### Database Operations
Database access is through `internal/database/`. Use transactions for multi-step operations.

### Configuration
All config is via environment variables. See `internal/config/config.go` for available options.

## Testing Guidelines

- Unit tests go alongside the code they test (e.g., `foo_test.go`)
- Integration tests go in `test/`
- Set `ARTNET_ENABLED=false` for CI environments
- Use table-driven tests where appropriate

## CI/CD

- **CI Workflow** (`ci.yml`): Runs tests, lint, and builds on PRs and pushes to main
- **Release Workflow** (`release.yml`): Manual workflow dispatch for creating releases

## Important Notes

- Always run `make lint` before committing
- All changes should go through PR, never commit directly to main
- GraphQL schema changes require regeneration with `make generate`
- Art-Net tests require UDP port 6454, which may conflict in some environments
