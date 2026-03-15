# DemiMine

A fully-isolated Minecraft proxy and server manager with a web UI, deployed via Docker Compose.

## Quick Summary

DemiMine manages multiple Minecraft backend servers (Paper, Purpur, Fabric, NeoForge, Forge) behind one or more Velocity proxies. Servers are dynamically started/stopped based on player activity, with lazy-loaded Java runtimes and plugin management via Modrinth.

**Tech Stack:** Go (backend) + SvelteKit (frontend) + TypeScript + SQLite + Docker

## Key Features

### ✅ Implemented

**Core Server Management:**
- Web UI accessible over LAN for headless server management
- Full CRUD operations for servers (create, read, update, delete)
- Dynamic server start/stop/restart via Docker
- Console log streaming with ANSI coloring
- Command execution with command history
- Full file browser with drag-drop upload, multi-file/folder support
- Code editor with syntax highlighting (Ace Editor)
- Image viewer for server files
- Icon upload for servers
- Backend servers on internal Docker network (not exposed to host)
- Port conflict detection with override option
- EULA auto-accepted on server creation

**Proxy System:**
- Multiple Velocity proxies supported (each in own container)
- CRUD operations for proxies
- Proxy console and file access
- Proxy icon upload
- Automatic velocity.toml sync when servers assigned/removed
- MiniMOTD integration for server list customization
- Plugin management for proxies

**Plugin Ecosystem:**
- Modrinth API integration
- Plugin search, install, update, uninstall
- Plugin version checking
- Plugin MC version filtering (global settings)

**File Operations:**
- Directory listing with breadcrumb navigation
- File content read/write
- File download
- File rename/delete
- Drag-and-drop upload with progress tracking
- Gzip decompression support
- Folder upload support

**Authentication & Security:**
- Admin setup on first launch
- JWT-based authentication
- Session management
- API key management
- Rate limiting (500 req/15min)
- CORS handling

**Database:**
- SQLite with WAL mode
- Automatic migrations
- Connection pooling
- Comprehensive schema for servers, proxies, players, backups, plugins, command history, crash logs, API keys

**Docker Integration:**
- Multi-stage build process
- Container lifecycle management
- Log streaming from containers
- Docker event monitoring
- Java runtime lazy-loading (versions 8, 17, 21, 25 from Adoptium)
- Automatic Java version detection based on MC version

**Real-time Communication:**
- WebSocket hub for real-time updates
- Live console log streaming
- Server state updates

### ⏳ In Progress

**Auto-shutdown:**
- Auto-shutdown timer configuration in server settings
- Idle shutdown logic (configurable, 15-min default)
- Player activity tracking infrastructure in place

### 📋 Planned (Phases 5-6)

**Backup System:**
- Backup creation (scheduled and manual)
- Backup restoration
- Backup retention policy
- Backup storage management

**Advanced Features:**
- Scheduled start/stop
- Server duplication for safe version updates
- Server JAR update action (maintains MC version)
- Custom Velocity auth plugin
- Dynamic server registration plugin

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

**IMPORTANT:** After any code changes (fixes, additions, or modifications), you MUST rebuild the Docker images without cache and restart the containers:

```bash
docker compose build --no-cache
docker compose up -d
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
go test ./internal/mc/... -v
```

Test coverage:
- Version comparison logic
- NeoForge version extraction
- Mock API testing for Paper, Purpur, Fabric
- Supported/unsupported type testing
- Properties generation and manipulation

Total: 452 lines of tests covering critical mc package functions

## Git Workflow

**IMPORTANT:** Only make git commits when the user explicitly requests it. Do not automatically commit changes.

## Project Status

**Production-ready foundation.** Phases 1-4 are complete and fully functional.

### Completed (Phases 1-4):
- ✅ Foundation: Go backend, SQLite, Auth, Middleware
- ✅ Server Management: Version fetching, JAR download, Docker containers, console, files
- ✅ Web UI: SvelteKit frontend with TypeScript and Tailwind, full feature integration
- ✅ Proxy System: Velocity containers, proxy management, MiniMOTD integration

### In Progress (Phase 5):
- ⏳ Backup System: Infrastructure exists, needs implementation

### Planned (Phase 6):
- 📋 Advanced Features: Scheduled operations, server duplication, JAR updates, custom plugins

### Statistics:
- **Backend**: 10,568 lines of Go code
- **Frontend**: SvelteKit 2.0, TypeScript 5.0, Tailwind 3.4
- **API Endpoints**: 10 handlers covering all features
- **Database**: 9 tables with automatic migrations
- **Docker**: Multi-stage build with Alpine runtime
- **Tests**: 452 lines of unit tests
- **Git**: 44 commits, actively developed

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