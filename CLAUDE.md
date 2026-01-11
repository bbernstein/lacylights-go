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

**Important:** After modifying `internal/graph/schema.graphqls`, always run `make generate` to update resolver stubs.

## Architecture

### Package Structure

```
lacylights-go/
├── cmd/server/               # Main application entry point
├── internal/
│   ├── config/               # Configuration via environment variables
│   ├── database/             # SQLite database layer with migrations
│   ├── graph/                # GraphQL schema, resolvers, generated code
│   │   ├── schema.graphqls   # GraphQL schema definition (source of truth)
│   │   ├── resolver.go       # Resolver implementations
│   │   └── generated/        # Auto-generated GraphQL code
│   ├── models/               # Domain models and business logic
│   ├── artnet/               # Art-Net protocol implementation for DMX
│   └── fade/                 # Fade engine for smooth transitions
├── pkg/                      # Public packages for external use
└── test/                     # Integration and end-to-end tests
```

### Key Technologies

- **GraphQL**: gqlgen for type-safe GraphQL API
- **Database**: SQLite with custom ORM layer
- **Art-Net**: UDP-based DMX protocol (port 6454)
- **WebSocket**: Real-time subscriptions for live updates

## Important Patterns

### GraphQL Schema
The GraphQL schema at `internal/graph/schema.graphqls` is the **source of truth** for API contracts. When making schema changes:
1. Edit `schema.graphqls`
2. Run `make generate`
3. Implement new resolvers in `internal/graph/resolver.go`
4. Update any affected types in `internal/models/`

### Database Operations
- All database access goes through `internal/database/`
- Use transactions for multi-step operations
- Migrations are in `internal/database/migrations/`

### Error Handling
- Return structured GraphQL errors for client-facing issues
- Log internal errors with context
- Use `fmt.Errorf("context: %w", err)` for error wrapping

### Fade Engine
The fade engine (`internal/fade/`) handles smooth DMX transitions:
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
| `ENV` | `development` | Environment (development/production) |
| `DATABASE_URL` | `file:./dev.db` | SQLite database path |
| `ARTNET_ENABLED` | `true` | Enable Art-Net DMX output |
| `ARTNET_PORT` | `6454` | Art-Net UDP port |
| `ARTNET_BROADCAST` | `""` | Broadcast address for Art-Net |
| `DMX_UNIVERSE_COUNT` | `4` | Number of DMX universes |
| `DMX_REFRESH_RATE` | `60` | DMX refresh rate (Hz) |
| `FADE_UPDATE_RATE` | `60` | Fade engine update rate (Hz) |
| `CORS_ORIGIN` | `http://localhost:3000` | Allowed CORS origin |
| `OFL_IMPORT_ENABLED` | `true` | Auto-import OFL fixtures on startup |
| `NON_INTERACTIVE` | `false` | Disable interactive prompts (CI/Docker) |

### Services and Ports

| Service | Port | Protocol |
|---------|------|----------|
| GraphQL API | 4000 (dev) / 4001 (test) | HTTP |
| WebSocket | 4000 | WS |
| Art-Net | 6454 | UDP |

## Related Repositories

| Repository | Relationship |
|------------|--------------|
| [lacylights-fe](https://github.com/bbernstein/lacylights-fe) | Frontend consumes this GraphQL API |
| [lacylights-mcp](https://github.com/bbernstein/lacylights-mcp) | MCP server calls this API for AI features |
| [lacylights-test](https://github.com/bbernstein/lacylights-test) | Integration tests validate API contracts |
| [lacylights-rpi](https://github.com/bbernstein/lacylights-rpi) | Deploys this backend to Raspberry Pi |
| [lacylights-mac](https://github.com/bbernstein/lacylights-mac) | macOS app embeds and manages this server |

## Important Notes

- Always run `make lint` before committing
- All changes go through PR, never commit directly to main
- GraphQL schema changes require `make generate`
- Art-Net tests require UDP port 6454 (may conflict in some environments)
- The GraphQL schema is the contract - changes affect all consumers

## Project Documentation

Key planning documents in the parent `lacylights/docs/` directory:
- **RASPBERRY_PI_PRODUCT_PLAN.md** - Hardware product architecture
- **GO_DISTRIBUTION_PLAN.md** - Binary distribution and releases
- **CONTRACT_TESTING_PLAN.md** - API contract testing strategy
