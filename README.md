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

- Go 1.25 or later
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
| `EXPORT_MAX_ROWS` | unset (unlimited) | Max data rows in one CSV/XLSX data export (a table, or a workbook's total) |
| `EXPORT_MAX_CELLS` | unset (unlimited) | Max cells in a dynamic-width `LOOK_DETAILS` grid |
| `EXPORT_MAX_BYTES` | unset (unlimited) | Max encoded size in bytes of any export: CSV/XLSX data exports and project JSON / Eos ASCII exports |
| `EXPORT_MAX_CONCURRENT_PROJECT_EXPORTS` | unset (built-in `2`) | How many whole-project (JSON or Eos) exports may run at once |

#### Export limits (opt-in)

Export limits are **off by default**. LacyLights is mainly used in-house, where
large projects must always export quickly, so an unset, `0`, negative or
non-numeric `EXPORT_MAX_*` value means "no limit". Only set them for a restricted
deployment such as a public demo. Suggested demo values:

```bash
EXPORT_MAX_ROWS=100000        # 100k rows
EXPORT_MAX_CELLS=5000000      # 5M cells
EXPORT_MAX_BYTES=20971520     # 20 MiB (the GraphQL response carries base64, ~1/3 larger)
EXPORT_MAX_CONCURRENT_PROJECT_EXPORTS=1
```

An export over a configured limit fails with a GraphQL error such as
`export is too large: LOOKS has about 150000 rows, over this server's export limit of 100000 rows`
or `export file is too large: project JSON export is 25000000 bytes, over this server's export limit of 20971520 bytes`.
`EXPORT_MAX_BYTES` takes a plain byte count (no `MB` suffix). The startup banner
prints the active limits (`Export caps: none (unlimited)` by default).

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

- [lacylights-fe](https://github.com/bbernstein/lacylights-fe) - Next.js frontend
- [lacylights-mcp](https://github.com/bbernstein/lacylights-mcp) - MCP server for AI integration
- [lacylights-test](https://github.com/bbernstein/lacylights-test) - Contract test suite
- [lacylights-rpi](https://github.com/bbernstein/lacylights-rpi) - Raspberry Pi deployment

## License

MIT License - See [LICENSE](LICENSE) for details.
