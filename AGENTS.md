# DemiMine

A fully-isolated Minecraft proxy and server manager with a web UI, deployed via Docker Compose.

## Quick Summary

DemiMine manages multiple Minecraft backend servers (Paper, Purpur, Fabric, NeoForge, Forge) behind one or more Velocity proxies. Servers are dynamically started/stopped based on player activity, with lazy-loaded Java runtimes and plugin management via Modrinth.

**Tech Stack:** Go (backend) + SvelteKit (frontend) + TypeScript + SQLite + Docker

## Key Features

### ✅ Implemented

**Core Server Management:**
- Admin setup on first launch
- JWT-based authentication with proper algorithm validation
- Session management
- API key management (used by DemiDynamic plugin)
- Rate limiting (500 req/15min) with IP spoofing protection
- Path exclusions from rate limiting (/health, /api/ws, /api/system/resources)
- CORS handling
- RCON password required at startup
- Config validation at startup

**Database:**
- SQLite with WAL mode
- Versioned automatic migrations with error logging
- Connection pooling (5min lifetime for WAL checkpointing)
- Comprehensive schema for servers, proxies, players, backups, plugins, command history, crash logs, API keys

**Docker Integration:**
- Multi-stage build process with HEALTHCHECK
- Container lifecycle management
- Log streaming from containers
- Docker event monitoring (start, stop, die, destroy, pause, unpause)
- Java runtime lazy-loading (versions 8, 17, 21, 25 from Adoptium)
- Automatic Java version detection based on MC version
- Configurable timezone via TZ env var

**Real-time Communication:**
- WebSocket hub for real-time updates
- Live console log streaming
- Server state updates
- Backup progress notifications

**Backup System:**
- Full-system backup (data + servers, optional java)
- Configurable schedule and retention policy
- Restore from disk or upload
- Backup management UI (settings page)

**Advanced Features:**
- Scheduled server start/stop
- JAR update system (Paper/Purpur/NanoLimbo/Velocity)
- Spark profiler integration
- Graceful shutdown with SIGTERM handling

## Project Status

**Production-ready.** Phases 1-6 are complete and fully functional.

### Completed (Phases 1-6):
- ✅ Foundation: Go backend, SQLite, Auth, Middleware
- ✅ Server Management: Version fetching, JAR download, Docker containers, console, files
- ✅ Web UI: SvelteKit frontend with TypeScript and Tailwind, full feature integration
- ✅ Proxy System: Velocity containers, proxy management, MiniMOTD integration
- ✅ Advanced Features: Scheduled start/stop, JAR updates, Spark profiler, auto-shutdown
- ✅ Backup System: Full-system backup/restore with scheduling and retention
- ✅ Production Hardening: HEALTHCHECK, config validation, migration versioning, error handling

### Statistics:
- **Backend**: 10,800+ lines of Go code
- **Frontend**: SvelteKit 2.0, TypeScript 5.0, Tailwind 3.4
- **API Endpoints**: 10+ handlers covering all features
- **Database**: 10 tables with versioned automatic migrations
- **Docker**: Multi-stage build with Alpine runtime and HEALTHCHECK
- **Tests**: 770+ lines of unit tests
- **Git**: 45+ commits, actively developed

## Architecture

```
┌────────────────────────────────────────────────────────────────┐
│  Docker Network: demimine_internal                             │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│  │ Paper    │  │ Fabric   │  │ NeoForge │ ... (not exposed)  │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                     │
│       └─────────────┼─────────────┘                           │
│                     │                                          │
│              ┌──────┴──────┐                                   │
│  ┌──────────┐│  Velocity   │←┐                               │
│  │ MiniMOTD ││  Proxy 1    │ │ Exposed to host (:25565)      │
│  └──────────┘└─────────────┘ │                               │
│               ┌──────────────┐│                              │
│               │  Velocity   │ │                              │
│               │  Proxy 2    │─┘                              │
│               └─────────────┘                                │
└────────────────────────────────────────────────────────────────┘
                      ▲
                      │ Docker Socket
┌─────────────────────┴────────────────────────────────────────┐
│  Manager Container                                           │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ Go API (Chi router, 10,568 lines)                     │  │
│  │ - 10 API handlers (servers, proxies, files, plugins, │  │
│  │   auth, versions, settings, WebSocket, Modrinth)     │  │
│  │ - Middleware (auth, CORS, rate limiting, logging)    │  │
│  │ - Docker client & container management               │  │
│  │ - WebSocket hub & console streaming                 │  │
│  │ - Modrinth integration                              │  │
│  │ - MiniMOTD integration                             │  │
│  │ - Plugin management                                │  │
│  └──────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ SvelteKit Frontend (TypeScript, Tailwind)           │  │
│  │ - Main canvas with pan/zoom                         │  │
│  │ - Server & proxy detail pages                       │  │
│  │ - Console, Files, Settings, Backups tabs            │  │
│  │ - Plugin browser                                    │  │
│  │ - Authentication pages                              │  │
│  └──────────────────────────────────────────────────────┘  │
│  ┌──────────────────────────────────────────────────────┐  │
│  │ SQLite Database (9 tables)                          │  │
│  │ - proxies, servers, players, api_keys, settings     │  │
│  │ - backups, crash_logs, command_history,             │  │
│  │   admin_auth, installed_plugins                      │  │
│  └──────────────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────────────────┘
```

## Implementation Details

See [design.md](./design.md) for the original implementation plan including:
- Complete API specification
- Database schema
- Docker integration details
- Web UI design
- Velocity proxy integration
- Backup system logic
- Security implementation

See [PROGRESS.md](./PROGRESS.md) for current development progress and phase status.

## Getting Started

```bash
# Build and start all services
docker compose up -d

# First time setup
# Open http://localhost:8080
# Set admin username and password
# Create a proxy, then create servers assigned to it
```

## Project Structure

```
DemiMine/
├── manager/                    # Go backend
│   ├── cmd/server/            # Entry point
│   └── internal/
│       ├── api/               # API handlers & middleware
│       ├── config/            # Configuration management
│       ├── db/                # SQLite database
│       ├── docker/            # Docker client & container management
│       ├── mc/                # Minecraft server logic
│       ├── models/            # Data models
│       ├── modrinth/          # Modrinth API client
│       ├── plugin/            # Plugin management
│       ├── websocket/         # WebSocket hub
│       ├── minimotd/          # MiniMOTD integration
│       └── java/              # Java runtime management
├── webui/                     # SvelteKit frontend
│   └── src/
│       ├── lib/
│       │   ├── components/    # UI components
│       │   ├── stores/        # Svelte stores
│       │   └── api.ts         # API client
│       └── routes/            # SvelteKit pages
├── data/                      # SQLite database
├── servers/                   # Server data directories
├── proxies/                   # Proxy data directories
├── backups/                   # Backup storage
├── java/                      # Lazy-loaded Java runtimes
├── docker-compose.yml         # Docker deployment
└── Dockerfile                 # Multi-stage build
```

## Development Workflow

**IMPORTANT:** After ANY code change, you MUST ALWAYS rebuild and restart the containers immediately. Never wait for the user to ask — just do it:

```bash
docker compose build --no-cache && docker compose up -d
```

This is critical because:
- The Go backend is compiled into the Docker image during build
- The SvelteKit frontend is bundled during build
- Container restarts alone will NOT pick up code changes
- Using `--no-cache` ensures all changes are included, not just the modified layer

### Development without Docker

```bash
# Backend (Go)
cd manager
go run ./cmd/server

# Frontend (SvelteKit)
cd webui
npm run dev
```

## Testing

Unit tests exist for core functionality:

```bash
cd manager
go test ./internal/... -v
```

Test coverage:
- Version comparison logic
- NeoForge version extraction
- Mock API testing for Paper, Purpur, Fabric
- Supported/unsupported type testing
- Properties generation and manipulation
- Backup creation and restore logic

## Git Workflow

**IMPORTANT:** Only make git commits when the user explicitly requests it. Do not automatically commit changes.

## Code Style & Conventions

**Backend (Go):**
- Standard Go formatting
- Chi router for API routing
- modernc.org/sqlite for SQLite (CGO-free)
- gorilla/websocket for WebSocket

**Frontend (SvelteKit):**
- TypeScript strict mode
- Svelte stores for state management
- Tailwind CSS for styling
- Ace Editor for code editing
- ansi_up for console coloring

**Comments:**
- Match casual style of existing comments
- Keep explanations clear and concise