# CLAUDE.md

This file provides guidance to Claude Code when working with code in this repository.

## Project Overview

LacyLights Go is the backend server for the LacyLights theatrical lighting control system. It provides a GraphQL API for managing fixtures, scenes, cue lists, and real-time DMX output via Art-Net protocol.

**Role in LacyLights Ecosystem:**
- Primary backend API consumed by lacylights-fe (frontend) and lacylights-mcp (AI integration)
- Owns the GraphQL schema that defines all API contracts
- Handles all DMX/Art-Net output to physical lighting fixtures
- Manages SQLite database for persistent storage

## Development Commands

### Building
```bash
make build                    # Build the server binary
make build-all                # Build for all platforms (linux/darwin, amd64/arm64)
go build ./cmd/server         # Direct Go build
```

### Testing
```bash
make test                     # Run all tests
make test-coverage            # Run tests with coverage report
go test -v ./...              # Run tests directly
ARTNET_ENABLED=false go test ./...  # Run tests without Art-Net
```

### Linting
```bash
make lint                     # Run golangci-lint
golangci-lint run             # Direct lint command
```

### Code Generation
```bash
make generate                 # Generate GraphQL resolver code
go generate ./...             # Direct generate command
```

**Important:** After modifying `internal/graphql/schema/schema.graphql`, always run `make generate` to update resolver stubs.

## Architecture

### Package Structure

```
lacylights-go/
├── cmd/server/               # Main application entry point
├── internal/
│   ├── config/               # Configuration via environment variables
│   ├── database/             # SQLite database layer with migrations
│   │   ├── models/           # Domain models and business logic
│   │   └── repositories/     # Database access layer
│   ├── graphql/              # GraphQL schema, resolvers, generated code
│   │   ├── schema/           # GraphQL schema definition (source of truth)
│   │   ├── resolvers/        # Resolver implementations
│   │   └── generated/        # Auto-generated GraphQL code
│   └── services/             # Business logic services
│       ├── fade/             # Fade engine for smooth transitions
│       ├── dmx/              # DMX output management
│       ├── ofl/              # Open Fixture Library integration
│       └── playback/         # Cue playback engine
├── pkg/
│   └── artnet/               # Art-Net protocol implementation for DMX
└── test/                     # Integration and end-to-end tests
```

### Key Technologies

- **GraphQL**: gqlgen for type-safe GraphQL API
- **Database**: SQLite with custom ORM layer
- **Art-Net**: UDP-based DMX protocol (port 6454)
- **WebSocket**: Real-time subscriptions for live updates

## Important Patterns

### GraphQL Schema
The GraphQL schema at `internal/graphql/schema/schema.graphql` is the **source of truth** for API contracts. When making schema changes:
1. Edit `internal/graphql/schema/schema.graphql`
2. Run `make generate`
3. Implement new resolvers in `internal/graphql/resolvers/`
4. Update any affected types in `internal/database/models/`

### Database Operations
- All database access goes through `internal/database/`
- Use transactions for multi-step operations
- Database schema is managed via GORM AutoMigrate (in server startup and test setup code), not separate migration files

### Error Handling
- Return structured GraphQL errors for client-facing issues
- Log internal errors with context
- Use `fmt.Errorf("context: %w", err)` for error wrapping

### Fade Engine
The fade engine (`internal/services/fade/`) handles smooth DMX transitions:
- Supports per-channel fade behavior (fade vs. snap)
- Configurable update rate (default 60Hz)
- Handles concurrent scene activations

## Testing Guidelines

- Unit tests alongside code: `foo_test.go` next to `foo.go`
- Integration tests in `test/` directory
- Set `ARTNET_ENABLED=false` for CI environments
- Use table-driven tests for comprehensive coverage
- Mock external dependencies (Art-Net, time)

## CI/CD

| Workflow | File | Purpose |
|----------|------|---------|
| CI | `ci.yml` | Tests, lint, build on PRs and main pushes |
| Release | `release.yml` | Manual workflow for creating releases |
| OFL Refresh | `refresh-ofl.yml` | Update Open Fixture Library data |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4000` | HTTP server port |
| `ENV` | `development` | Environment (development/production/integration/e2e) |
| `DATABASE_URL` | (see below) | SQLite database path (defaults based on ENV) |
| `ARTNET_ENABLED` | `true` | Enable Art-Net DMX output |
| `ARTNET_PORT` | `6454` | Art-Net UDP port |
| `ARTNET_BROADCAST` | `""` | Broadcast address for Art-Net |
| `DMX_UNIVERSE_COUNT` | `4` | Number of DMX universes |
| `DMX_REFRESH_RATE` | `60` | DMX refresh rate (Hz) |
| `FADE_UPDATE_RATE` | `60` | Fade engine update rate (Hz) |
| `CORS_ORIGIN` | `http://localhost:3000` | Allowed CORS origin |
| `CORS_ALLOW_ALL` | `false` | Allow all CORS origins (E2E testing only) |
| `OFL_IMPORT_ENABLED` | `true` | Auto-import OFL fixtures on startup |
| `OFL_CACHE_PATH` | `./.ofl-cache` | Path to the OFL fixture cache directory |
| `NON_INTERACTIVE` | `false` | Disable interactive prompts (CI/Docker) |

### Database Environments

The database filename defaults based on the `ENV` environment variable. This allows different database files for different purposes, keeping production, development, and test data separate:

| ENV Value | Default Database | Purpose |
|-----------|-----------------|---------|
| `development` | `dev.db` | Local development (default) |
| `production` | `lacylights.db` | Production data |
| `integration` | `integration.db` | Integration tests (can be auto-cleaned) |
| `e2e` | `e2e.db` | End-to-end tests (can be auto-cleaned) |
| `test` | `dev.db` | Unit tests (typically use in-memory override) |

You can always override the default by setting `DATABASE_URL` explicitly:
```bash
# Run with integration database
ENV=integration ./lacylights-go

# Run with explicit database path (overrides ENV-based default)
DATABASE_URL=file:./custom.db ./lacylights-go
```

### Services and Ports

| Service | Port | Protocol |
|---------|------|----------|
| GraphQL API | 4000 (configurable via `PORT` env var) | HTTP |
| WebSocket | 4000 | WS |
| Art-Net | 6454 | UDP |

## Related Repositories

| Repository | Relationship |
|------------|--------------|
| [lacylights-fe](https://github.com/bbernstein/lacylights-fe) | Frontend consumes this GraphQL API |
| [lacylights-mcp](https://github.com/bbernstein/lacylights-mcp) | MCP server calls this API for AI features |
| [lacylights-test](https://github.com/bbernstein/lacylights-test) | Integration tests validate API contracts |
| [lacylights-terraform](https://github.com/bbernstein/lacylights-terraform) | Distribution infrastructure - releases uploaded here |
| [lacylights-rpi](https://github.com/bbernstein/lacylights-rpi) | Production platform - hosts this backend on Raspberry Pi |
| [lacylights-mac](https://github.com/bbernstein/lacylights-mac) | Production platform - hosts this backend on macOS |

## Important Notes

- Always run `make lint` before committing
- All changes go through PR, never commit directly to main
- GraphQL schema changes require `make generate`
- Art-Net tests require UDP port 6454 (may conflict in some environments)
- The GraphQL schema is the contract - changes affect all consumers

## Project Documentation

Key planning documents in the parent directory (e.g., `../RASPBERRY_PI_PRODUCT_PLAN.md`):
- **LACYLIGHTS_GO_REWRITE_PLAN.md** - Go backend rewrite and architecture plan
- **RASPBERRY_PI_PRODUCT_PLAN.md** - Hardware product architecture
- **GO_DISTRIBUTION_PLAN.md** - Binary distribution and releases
- **CONTRACT_TESTING_PLAN.md** - API contract testing strategy
