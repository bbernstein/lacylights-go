# LacyLights Go Server

A high-performance Go implementation of the LacyLights lighting control system backend. This server provides a GraphQL API for managing theatrical lighting fixtures, scenes, cue lists, and real-time DMX output via Art-Net.

## Features

- **GraphQL API**: Full-featured API for lighting control operations
- **Art-Net Support**: Real-time DMX output over UDP (Art-Net protocol)
- **SQLite Database**: Lightweight, embedded database using Prisma-style schema
- **Cross-Platform**: Builds for Linux (ARM/AMD64), macOS, and Windows
- **Raspberry Pi Ready**: Optimized for deployment on Raspberry Pi hardware

## Quick Start

### Prerequisites

- Go 1.21 or later
- Make (optional, for convenience commands)

### Build and Run

```bash
# Build the server
make build

# Run with default settings
./build/bin/server

# Run with custom settings
DATABASE_URL="file:./data.db" PORT=4000 ./build/bin/server
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4000` | HTTP server port |
| `DATABASE_URL` | `file:./lacylights.db` | SQLite database path |
| `ARTNET_ENABLED` | `true` | Enable/disable Art-Net output |
| `ARTNET_BROADCAST_ADDRESS` | `255.255.255.255` | Art-Net broadcast address |

## Development

### Project Structure

```
lacylights-go/
├── cmd/server/          # Main application entry point
├── internal/
│   ├── config/          # Configuration management
│   ├── database/        # Database layer and migrations
│   ├── graph/           # GraphQL resolvers and schema
│   ├── models/          # Domain models
│   └── artnet/          # Art-Net protocol implementation
├── pkg/                 # Public packages
├── test/                # Integration tests
├── go.mod
├── go.sum
└── Makefile
```

### Common Commands

```bash
# Run tests
make test

# Run tests with coverage
make test-coverage

# Run linter
make lint

# Generate GraphQL code
make generate

# Build for all platforms
make build-all

# Clean build artifacts
make clean
```

### GraphQL Schema

The GraphQL API is defined in `internal/graph/schema.graphqls`. After modifying the schema, regenerate the resolver code:

```bash
make generate
```

## API Overview

### Queries

- `projects` - List all projects
- `project(id: ID!)` - Get a specific project
- `fixtureDefinitions` - List fixture definitions
- `systemInfo` - Get system status and configuration
- `dmxOutput(universe: Int!)` - Get current DMX channel values

### Mutations

- `createProject` / `updateProject` / `deleteProject` - Project management
- `createScene` / `updateScene` / `deleteScene` - Scene management
- `createCueList` / `addCueToCueList` - Cue list management
- `startCueList` / `nextCue` / `stopCueList` - Playback control
- `setChannelValue` - Direct DMX control
- `fadeToBlack` - Emergency blackout

### Subscriptions

- `dmxOutput` - Real-time DMX value updates
- `playbackStatus` - Cue list playback state changes

## Building for Raspberry Pi

```bash
# Build for Raspberry Pi (ARM64)
GOOS=linux GOARCH=arm64 go build -o lacylights-arm64 ./cmd/server

# Build for older Raspberry Pi (ARM32)
GOOS=linux GOARCH=arm GOARM=7 go build -o lacylights-arm ./cmd/server
```

## Testing

Run unit tests:
```bash
make test
```

Run with coverage report:
```bash
make test-coverage
open coverage.html
```

## Project Documentation

For comprehensive implementation plans and architecture decisions, see the parent directory documentation:

- [LACYLIGHTS_GO_REWRITE_PLAN.md](../LACYLIGHTS_GO_REWRITE_PLAN.md) - Complete Go rewrite implementation plan and architecture
- [RASPBERRY_PI_PRODUCT_PLAN.md](../RASPBERRY_PI_PRODUCT_PLAN.md) - Turnkey Raspberry Pi product architecture and deployment
- [GO_DISTRIBUTION_PLAN.md](../GO_DISTRIBUTION_PLAN.md) - Binary distribution and release workflow coordination
- [CONTRACT_TESTING_PLAN.md](../CONTRACT_TESTING_PLAN.md) - Contract test suite for API compatibility verification

## Related Repositories

- [lacylights-node](https://github.com/bbernstein/lacylights-node) - Node.js/TypeScript backend (original)
- [lacylights-fe](https://github.com/bbernstein/lacylights-fe) - Next.js frontend
- [lacylights-mcp](https://github.com/bbernstein/lacylights-mcp) - MCP server for AI integration
- [lacylights-test](https://github.com/bbernstein/lacylights-test) - Contract test suite

## License

MIT License - See [LICENSE](LICENSE) for details.
