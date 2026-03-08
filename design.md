# DemiMine - Design Document

A fully-isolated Minecraft proxy and server manager with a web UI, deployed via Docker Compose.

## Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Tech Stack](#tech-stack)
4. [Project Structure](#project-structure)
5. [Database Schema](#database-schema)
6. [API Specification](#api-specification)
7. [Docker Integration](#docker-integration)
8. [Java Version Management](#java-version-management)
9. [Minecraft Server Support](#minecraft-server-support)
10. [Web UI Design](#web-ui-design)
11. [Velocity Proxy Integration](#velocity-proxy-integration)
12. [Backup System](#backup-system)
13. [Security](#security)
14. [Implementation Phases](#implementation-phases)

---

## Overview

### Goals

- Single-command deployment via Docker Compose
- Web UI accessible over LAN for headless server management
- Dynamic server start/stop based on player activity
- Automatic Java version management
- Full file browser and console access via Web UI
- Integrated backup system with restoration
- Proxy-based authentication for server access

### Non-Goals

- Multi-user admin system (single admin only)
- Public internet exposure (LAN only)
- Client modpack import (server-side only)
- Auto-restart on crash (manual intervention preferred)
- External notifications (manual monitoring)

---

## Architecture

```
┌────────────────────────────────────────────────────────────────────────┐
│  Docker Network: demimine_internal (bridge)                            │
│  ┌──────────────────────────────────────────────────────────────────┐  │
│  │                                                                   │  │
│  │  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │  │
│  │  │ Paper       │  │ Fabric      │  │ NeoForge    │  ...         │  │
│  │  │ :25565      │  │ :25565      │  │ :25565      │              │  │
│  │  │ (internal)  │  │ (internal)  │  │ (internal)  │              │  │
│  │  └─────────────┘  └─────────────┘  └─────────────┘              │  │
│  │         ▲                ▲                ▲                      │  │
│  │         │                │                │                      │  │
│  │         └────────────────┼────────────────┘                      │  │
│  │                          │                                       │  │
│  │                   ┌──────┴──────┐                                │  │
│  │                   │  Velocity   │                                │  │
│  │                   │  Proxy(s)   │                                │  │
│  │                   │  :25577     │                                │  │
│  │                   └──────┬──────┘                                │  │
│  └──────────────────────────┼───────────────────────────────────────┘  │
│                              │                                          │
└──────────────────────────────┼──────────────────────────────────────────┘
                               │
                        ┌──────┴──────┐
                        │   Player    │
                        │  (external) │
                        └─────────────┘

┌────────────────────────────────────────────────────────────────────────┐
│  Manager Container (separate, manages all via Docker socket)           │
│  ├─ API Server (REST, port configurable)                               │
│  ├─ WebSocket Server (real-time updates)                               │
│  ├─ Web UI (served as static files)                                    │
│  ├─ Docker SDK (creates/destroys server & proxy containers)            │
│  └─ SQLite (all persistent state)                                      │
│                                                                         │
│  Volume Mounts:                                                         │
│  ├─ ./data:/data          → SQLite DB, manager config                  │
│  ├─ ./servers:/servers    → All backend server files                   │
│  ├─ ./proxies:/proxies    → All proxy files (velocity.toml, plugins)   │
│  ├─ ./backups:/backups    → Backup storage (bind mount)                │
│  ├─ ./java:/java          → Lazy-loaded Java runtimes                  │
│  └─ /var/run/docker.sock  → Docker API access                         │
└────────────────────────────────────────────────────────────────────────┘
```

### Key Architectural Decisions

1. **Velocity as separate container(s)** - Proxies run in their own containers, not in the manager. This allows multiple proxies if needed.

2. **Internal network for proxied servers** - Backend servers assigned to a proxy are NOT exposed to the host. They communicate only via the internal Docker network. Players must go through the proxy to reach them.

3. **Standalone server support** - Servers can be created without a proxy assignment. Standalone servers ARE exposed to the host on their assigned port, allowing direct player connections. Useful for testing, single-server setups, or non-Minecraft-proxy use cases.

4. **Flexible proxy count** - The system supports multiple Velocity proxies if desired (e.g., different auth modes, forced hosts, etc.). Each proxy container connects to the same internal network.

### Container Communication Flow

```
Player → Velocity Proxy (:25565 exposed) → Backend Server (internal :25565)
                    │
                    └─── If server stopped ───→ DemiMine Manager API
                                                        ↓
                                              Manager starts container
                                                        ↓
                                              Proxy connects player
```

### Request Flow (with proxy auth)

1. Player connects to Velocity proxy (exposed port)
2. Auth plugin intercepts, sends to limbo
3. Player enters password, auth plugin validates
4. On success, player sent to nexus (hub)
5. Player selects backend server
6. Dynamic plugin checks if server running
7. If not, requests manager API to start
8. Manager creates/starts container on internal network
9. Player transferred to backend server

### Request Flow (standalone server)

1. Manager creates server without proxy assignment
2. Server is exposed to host on assigned port (e.g., :25565)
3. Players connect directly to host:port (no proxy, no auth)
4. Server can be started/stopped manually via API/UI
5. Useful for development, testing, or simple single-server deployments

---

## Tech Stack

| Layer | Technology | Justification |
|-------|------------|---------------|
| Backend | Go 1.21+ | Single binary, low memory, excellent Docker SDK |
| Router | Chi | Lightweight, standard library compatible |
| WebSocket | gorilla/websocket | Mature, well-documented |
| Database | SQLite + modernc.org/sqlite | Pure Go, no CGO required |
| Frontend | SvelteKit | Lightweight, fast, reactive |
| CSS | Tailwind | Rapid development, small bundle |
| Docker SDK | docker/docker | Official Go SDK |
| Java Source | Adoptium API | Open-source, legal to redistribute |

---

## Project Structure

```
DemiMine/
├── docker-compose.yml
├── Dockerfile
├── README.md
├── AGENTS.md
├── design.md
│
├── manager/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go           # Entry point
│   ├── internal/
│   │   ├── api/
│   │   │   ├── router.go         # Route definitions
│   │   │   ├── handlers/         # Endpoint handlers
│   │   │   │   ├── auth.go
│   │   │   │   ├── servers.go
│   │   │   │   ├── proxies.go
│   │   │   │   ├── files.go
│   │   │   │   ├── backups.go
│   │   │   │   ├── java.go
│   │   │   │   ├── keys.go
│   │   │   │   └── system.go
│   │   │   └── middleware/
│   │   │       ├── auth.go
│   │   │       └── logging.go
│   │   ├── ws/
│   │   │   ├── hub.go            # WebSocket connection manager
│   │   │   └── handlers.go       # Message handlers
│   │   ├── docker/
│   │   │   ├── client.go         # Docker client wrapper
│   │   │   ├── containers.go     # Container lifecycle
│   │   │   ├── server.go         # Server container templates
│   │   │   └── proxy.go          # Proxy container templates
│   │   ├── java/
│   │   │   ├── manager.go        # Java version management
│   │   │   ├── download.go       # Adoptium API integration
│   │   │   └── detect.go         # Version detection
│   │   ├── mc/
│   │   │   ├── versions.go       # MC version fetching
│   │   │   ├── download.go       # Server jar download
│   │   │   └── properties.go     # server.properties handling
│   │   ├── backup/
│   │   │   ├── scheduler.go      # Backup scheduling
│   │   │   ├── create.go         # Backup creation
│   │   │   └── restore.go        # Backup restoration
│   │   ├── db/
│   │   │   ├── db.go             # Database connection
│   │   │   ├── migrations.go     # Schema migrations
│   │   │   └── queries/          # SQL queries
│   │   │       ├── servers.go
│   │   │       ├── proxies.go
│   │   │       ├── players.go
│   │   │       ├── backups.go
│   │   │       └── settings.go
│   │   └── config/
│   │       ├── config.go         # Config loading
│   │       └── defaults.go       # Default values
│   ├── pkg/
│   │   ├── auth/
│   │   │   ├── password.go       # Hashing, validation
│   │   │   └── session.go        # JWT handling
│   │   └── utils/
│   │       └── files.go          # File utilities
│   ├── go.mod
│   └── go.sum
│
├── webui/
│   ├── src/
│   │   ├── lib/
│   │   │   ├── components/
│   │   │   │   ├── Console.svelte
│   │   │   │   ├── FileTree.svelte
│   │   │   │   ├── CodeEditor.svelte
│   │   │   │   ├── PlayerRow.svelte
│   │   │   │   ├── ServerCard.svelte
│   │   │   │   ├── CrashBanner.svelte
│   │   │   │   └── ResourceGraph.svelte
│   │   │   └── api.ts            # API client
│   │   ├── routes/
│   │   │   ├── +layout.svelte
│   │   │   ├── +page.svelte      # Dashboard
│   │   │   ├── login/
│   │   │   │   └── +page.svelte
│   │   │   ├── servers/
│   │   │   │   └── [id]/
│   │   │   │       ├── +page.svelte
│   │   │   │       ├── console/
│   │   │   │       ├── players/
│   │   │   │       ├── files/
│   │   │   │       ├── settings/
│   │   │   │       └── backups/
│   │   │   ├── proxy/
│   │   │   │   └── +page.svelte
│   │   │   └── admin/
│   │   │       ├── api-keys/
│   │   │       ├── java/
│   │   │       └── settings/
│   │   └── stores/
│   │       ├── servers.ts
│   │       ├── websocket.ts
│   │       └── auth.ts
│   ├── static/
│   ├── package.json
│   ├── svelte.config.js
│   ├── tailwind.config.js
│   └── vite.config.ts
│
├── velocity-plugins/
│   ├── auth-plugin/
│   │   ├── src/main/java/...
│   │   └── build.gradle
│   └── dynamic-plugin/
│       ├── src/main/java/...
│       └── build.gradle
│
└── scripts/
    ├── build.sh                  # Build all components
    └── dev.sh                    # Development setup
```

---

## Database Schema

### Tables

```sql
-- Proxy configurations (Velocity instances)
CREATE TABLE proxies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    host_port INTEGER NOT NULL UNIQUE,  -- Port exposed to host (e.g., 25565)
    status TEXT DEFAULT 'stopped',      -- stopped, running, starting
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Server configurations
CREATE TABLE servers (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL,           -- paper, purpur, fabric, neoforge, forge
    version TEXT NOT NULL,        -- e.g., "1.21.1"
    proxy_id INTEGER,             -- Nullable: server can be standalone or assigned to proxy
    host_port INTEGER,            -- Nullable: port exposed to host (only for standalone servers)
    ram_mb INTEGER NOT NULL,
    domain TEXT,                  -- for forced hosts, e.g., "hardcore.example.com"
    icon_path TEXT,             -- Path to custom server icon image (nullable)
    backup_interval_days INTEGER DEFAULT 0,
    auto_shutdown_minutes INTEGER DEFAULT 15,
    scheduled_start TEXT,         -- HH:MM format, nullable
    scheduled_stop TEXT,          -- HH:MM format, nullable
    status TEXT DEFAULT 'stopped', -- stopped, running, starting, crashed
    canvas_x INTEGER DEFAULT 0,   -- X position on visual canvas
    canvas_y INTEGER DEFAULT 0,   -- Y position on visual canvas
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (proxy_id) REFERENCES proxies(id) ON DELETE SET NULL
);

-- Online players (cleared on manager restart)
CREATE TABLE players (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    uuid TEXT NOT NULL,
    server_id INTEGER NOT NULL,
    joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

-- API keys for plugin integration
CREATE TABLE api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    key_hash TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    last_used_at DATETIME
);

-- Manager settings
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

-- Installed Java versions
CREATE TABLE java_versions (
    version TEXT PRIMARY KEY,     -- e.g., "17", "21"
    path TEXT NOT NULL,
    downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Backup records
CREATE TABLE backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL,
    path TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    size_bytes INTEGER NOT NULL,
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

-- Crash logs
CREATE TABLE crash_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL,
    content TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    acknowledged INTEGER DEFAULT 0,
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

-- Command history per server
CREATE TABLE command_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL,
    command TEXT NOT NULL,
    executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
);

-- Admin credentials (single row)
CREATE TABLE admin_auth (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    username TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    setup_complete INTEGER DEFAULT 0
);
```

### Indexes

```sql
CREATE INDEX idx_proxies_status ON proxies(status);
CREATE INDEX idx_servers_status ON servers(status);
CREATE INDEX idx_servers_proxy ON servers(proxy_id);
CREATE INDEX idx_players_server ON players(server_id);
CREATE INDEX idx_players_uuid ON players(uuid);
CREATE INDEX idx_backups_server ON backups(server_id);
CREATE INDEX idx_crash_logs_server ON crash_logs(server_id);
CREATE INDEX idx_command_history_server ON command_history(server_id);
```

### Log Retention

| Log Type | Retention | Notes |
|----------|-----------|-------|
| Server logs | Indefinite | Stored in container, accessible via API |
| Crash logs | Indefinite | Stored in database, admin can clear |
| Command history | Indefinite | Stored in database, admin can clear |
| Manager logs | Indefinite | Written to `./data/manager.log` |

Admins can manually clear crash logs and command history via the API or Web UI as needed.

---

## API Specification

### Authentication

All endpoints except `/api/auth/*` require `Authorization: Bearer <token>` header or valid session cookie.

### Endpoints

#### Authentication

```
POST /api/auth/setup
  Body: { "username": string, "password": string }
  Response: { "success": true }
  Notes: Only available on first launch, sets admin credentials

POST /api/auth/login
  Body: { "username": string, "password": string }
  Response: { "token": string, "expires_at": string }
  Notes: Returns JWT token

POST /api/auth/logout
  Response: { "success": true }

GET /api/auth/status
  Response: { "setup_complete": boolean, "logged_in": boolean }
```

**First-Run Flow:**
1. Manager starts, checks if setup is complete
2. If not, UI shows setup form requesting username and password
3. Credentials stored (username for display, password hashed with bcrypt)
4. User redirected to login
5. Dashboard is empty - user creates proxy and servers as needed

#### Servers

```
GET /api/servers
  Response: [
    {
      "id": number,
      "name": string,
      "type": string,
      "version": string,
      "proxy_id": number | null,
      "proxy_name": string | null,
      "ram_mb": number,
      "domain": string | null,
      "backup_interval_days": number,
      "auto_shutdown_minutes": number,
      "scheduled_start": string | null,
      "scheduled_stop": string | null,
      "host_port": number | null,     -- Only set for standalone servers
      "status": string,
      "player_count": number,
      "created_at": string
    }
  ]

POST /api/servers
  Body: {
    "name": string,
    "type": string,           -- paper, purpur, fabric, neoforge, forge
    "version": string,
    "proxy_id": number | null,  -- Optional: assign to proxy for auto-start
    "host_port": number | null, -- Optional: expose to host (required if proxy_id is null)
    "ram_mb": number,
    "domain": string | null,
    "backup_interval_days": number,
    "auto_shutdown_minutes": number,
    "scheduled_start": string | null,
    "scheduled_stop": string | null
  }
  Response: { "id": number }
  Errors: 
    { "error": "port_in_use", "port": number, "used_by": string }
    Notes: |
      Port conflict warning - API returns error but allows override on retry.
      UI flow: Show warning "Port {port} is already used by {used_by}. Use anyway?"
      User must click save/create button a second time to confirm override.
      Pass `?force=true` query param to bypass the check.

PATCH /api/servers/:id
  Body: { ...fields to update }
  Response: { "success": true }
  Errors:
    { "error": "port_in_use", "port": number, "used_by": string }
    Notes: Same override behavior as POST - use `?force=true` to bypass

DELETE /api/servers/:id
  Response: { "success": true }
  Notes: Deletes server folder and all data

POST /api/servers/:id/start
  Response: { "success": true, "message": string }
  Errors: { "error": "insufficient_ram", "available_mb": number, "required_mb": number }

POST /api/servers/:id/stop
  Response: { "success": true }

POST /api/servers/:id/restart
  Response: { "success": true }

POST /api/servers/:id/duplicate
  Body: {
    "name": string,           -- New server name
    "host_port": number | null,  -- For standalone servers
    "ram_mb": number,
    "proxy_id": number | null
  }
  Response: { "id": number }
  Notes: Copies server files, plugins, worlds. Does not copy version (uses same as source).

POST /api/servers/:id/update-jar
  Response: { "success": true, "old_build": string, "new_build": string }
  Notes: Downloads latest server jar for the same MC version/type. Server must be stopped.

PATCH /api/servers/:id/position
  Body: { "canvas_x": number, "canvas_y": number }
  Response: { "success": true }
  Notes: Updates server position on visual canvas. Validates position doesn't overlap with other servers.

POST /api/servers/:id/icon
  Body: multipart/form-data (image file)
  Response: { "success": true, "icon_path": string }
  Notes: Uploads custom server icon. Accepts PNG/JPG, recommended 64x64 pixels.

DELETE /api/servers/:id/icon
  Response: { "success": true }
  Notes: Removes custom icon, reverts to default placeholder.
```

#### Server Console

```
GET /api/servers/:id/logs
  Query: ?lines=100&follow=false
  Response: { "logs": string[] }
  Notes: If follow=true, upgrades to WebSocket

POST /api/servers/:id/command
  Body: { "command": string }
  Response: { "success": true }

GET /api/servers/:id/history
  Query: ?limit=50
  Response: { "commands": [{ "command": string, "executed_at": string }] }
```

#### Server Files

```
GET /api/servers/:id/files
  Query: ?path=/
  Response: {
    "path": string,
    "entries": [
      { "name": string, "is_dir": boolean, "size": number, "modified": string }
    ]
  }

GET /api/servers/:id/files/content
  Query: ?path=/server.properties
  Response: { "content": string }
  Notes: Returns raw file content, 404 if binary

PUT /api/servers/:id/files/content
  Query: ?path=/server.properties
  Body: { "content": string }
  Response: { "success": true }

POST /api/servers/:id/files/upload
  Query: ?path=/plugins/
  Body: multipart/form-data (supports multiple files and directories)
  Response: { "success": true, "uploaded": ["filename1", "filename2", ...] }
  Notes: 
    - Supports multiple files/folders in single request
    - Preserves directory structure
    - Used for both drag-and-drop and file picker UI

DELETE /api/servers/:id/files
  Query: ?path=/old-plugin.jar
  Response: { "success": true }

POST /api/servers/:id/files/rename
  Body: { "old_path": string, "new_name": string }
  Response: { "success": true }

GET /api/servers/:id/files/download
  Query: ?path=/world/region/r.0.0.mca
  Response: application/octet-stream
```

#### Server Players

```
GET /api/servers/:id/players
  Response: {
    "players": [
      {
        "name": string,
        "uuid": string,
        "joined_at": string,
        "skin_url": string
      }
    ]
  }
```

#### Backups

```
GET /api/servers/:id/backups
  Response: {
    "backups": [
      {
        "id": number,
        "path": string,
        "created_at": string,
        "size_bytes": number,
        "size_human": string
      }
    ]
  }

POST /api/servers/:id/backups
  Response: { "success": true, "backup_id": number }

POST /api/servers/:id/backups/:backup_id/restore
  Response: { "success": true }
  Notes: Stops server, restores, does not restart

DELETE /api/servers/:id/backups/:backup_id
  Response: { "success": true }
```

#### Proxies (Velocity)

```
GET /api/proxies
  Response: [
    {
      "id": number,
      "name": string,
      "host_port": number,
      "status": string,
      "player_count": number,
      "connected_servers": string[],
      "created_at": string
    }
  ]

POST /api/proxies
  Body: {
    "name": string,
    "host_port": number       -- Port exposed to host (UI defaults to 25565)
  }
  Response: { "id": number }
  Errors:
    { "error": "port_in_use", "port": number, "used_by": string }
    Notes: Same override behavior as servers - use `?force=true` to bypass

GET /api/proxies/:id
  Response: {
    "id": number,
    "name": string,
    "host_port": number,
    "status": string,
    "player_count": number,
    "connected_servers": string[],
    "version": string
  }

PATCH /api/proxies/:id
  Body: { "host_port": number }  -- Can update exposed port
  Response: { "success": true }
  Errors:
    { "error": "port_in_use", "port": number, "used_by": string }
    Notes: Same override behavior - use `?force=true` to bypass

DELETE /api/proxies/:id
  Response: { "success": true }
  Notes: Deletes proxy container and files, unassigns associated servers

POST /api/proxies/:id/start
  Response: { "success": true }

POST /api/proxies/:id/stop
  Response: { "success": true }

POST /api/proxies/:id/restart
  Response: { "success": true }

GET /api/proxies/:id/config
  Response: { "content": string }

PUT /api/proxies/:id/config
  Body: { "content": string }
  Response: { "success": true }

GET /api/proxies/:id/plugins
  Response: [{ "name": string, "enabled": boolean, "version": string }]

POST /api/proxies/:id/plugins/upload
  Body: multipart/form-data
  Response: { "success": true, "filename": string }

DELETE /api/proxies/:id/plugins/:name
  Response: { "success": true }

POST /api/proxies/:id/update
  Body: { "confirm": true }
  Response: { "success": true, "old_version": string, "new_version": string }
```

#### Java Versions

```
GET /api/java
  Response: {
    "installed": [
      { "version": string, "path": string, "downloaded_at": string }
    ],
    "available": [17, 21]  -- versions that can be downloaded
  }

DELETE /api/java/:version
  Response: { "success": true }
```

#### Version Fetching

```
GET /api/versions/:type
  Response: {
    "versions": [
      { "version": string, "stable": boolean, "builds": number }
    ]
  }
  Notes: type = paper, purpur, fabric, neoforge
```

#### API Keys

```
GET /api/keys
  Response: [
    { "id": number, "name": string, "created_at": string, "last_used_at": string }
  ]

POST /api/keys
  Body: { "name": string }
  Response: { "id": number, "key": string }
  Notes: Key only shown once!

DELETE /api/keys/:id
  Response: { "success": true }
```

#### System

```
GET /api/system/resources
  Response: {
    "total_memory_mb": number,
    "used_memory_mb": number,
    "available_memory_mb": number,
    "cpu_percent": number,
    "servers_running": number,
    "servers_allocated_mb": number
  }

GET /api/crashes
  Response: [
    {
      "id": number,
      "server_name": string,
      "server_id": number,
      "created_at": string,
      "acknowledged": boolean,
      "summary": string
    }
  ]

GET /api/crashes/:id
  Response: { "content": string, "server_name": string, "created_at": string }

POST /api/crashes/:id/acknowledge
  Response: { "success": true }
```

#### WebSocket

```
WS /api/ws
  Auth: Query param ?token=xxx or header

  Client → Server:
    { "type": "subscribe", "channel": "servers" }
    { "type": "subscribe", "channel": "server:1" }
    { "type": "unsubscribe", "channel": "..." }
    { "type": "command", "server_id": 1, "command": "list" }

  Server → Client:
    { "type": "server_status", "server_id": 1, "status": "running" }
    { "type": "player_join", "server_id": 1, "player": { "name": "...", "uuid": "..." } }
    { "type": "player_leave", "server_id": 1, "player": { "name": "..." } }
    { "type": "log", "server_id": 1, "line": "...", "level": "INFO" }
    { "type": "resources", "server_id": 1, "cpu_percent": 5.2, "memory_mb": 1024 }
    { "type": "crash", "server_id": 1, "crash_log_id": 5 }
```

---

## Docker Integration

### Manager Container

```dockerfile
# Dockerfile
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY manager/go.mod manager/go.sum ./
RUN go mod download
COPY manager/ ./
RUN CGO_ENABLED=0 go build -o /demimine ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /demimine /usr/local/bin/demimine
COPY webui/build/ /app/webui/
WORKDIR /app
EXPOSE 8080
CMD ["demimine"]
```

### Docker Compose

```yaml
# docker-compose.yml
version: "3.8"

networks:
  demimine_internal:
    driver: bridge

services:
  manager:
    build: .
    container_name: demimine-manager
    restart: unless-stopped
    ports:
      - "${WEBUI_PORT:-8080}:8080"
    volumes:
      - ./data:/data
      - ./servers:/servers
      - ./proxies:/proxies
      - ./backups:/backups
      - ./java:/java
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - DEMIMINE_PORT=8080
      - DEMIMINE_DATA_DIR=/data
      - DEMIMINE_SERVERS_DIR=/servers
      - DEMIMINE_PROXIES_DIR=/proxies
      - DEMIMINE_BACKUPS_DIR=/backups
      - DEMIMINE_JAVA_DIR=/java
      - DEMIMINE_MAX_RAM_MB=16384
      - DEMIMINE_NETWORK=demimine_internal
    labels:
      - "demimine.managed=false"
    networks:
      - demimine_internal

  # Velocity proxy(s) are created dynamically by the manager
  # Example of what manager creates:
  # velocity-main:
  #   image: itzg/mc-proxy
  #   container_name: demimine-velocity-main
  #   ports:
  #     - "25565:25577"
  #   volumes:
  #     - ./proxies/main:/server
  #   networks:
  #     - demimine_internal
  #   labels:
  #     demimine.managed: "true"
  #     demimine.type: "proxy"
```

### Server Container Template

Each backend server runs in its own container on the internal network. Port exposure depends on proxy assignment:

- **Proxied servers**: NOT exposed to host - only accessible via proxy
- **Standalone servers**: Exposed to host on assigned port - direct player access

```go
type ServerContainerConfig struct {
    Name         string
    ServerType   string
    Version      string
    RAMMB        int
    JavaVersion  string
    ServerPath   string  // /servers/{name}
    JavaPath     string  // /java/{version}
    NetworkName  string  // demimine_internal
    HostPort     int     // Port exposed to host (0 = not exposed, for proxied servers)
}

func CreateServerContainer(cfg ServerContainerConfig) (*container.Container, error) {
    config := &container.Config{
        Image: "eclipse-temurin:21-jre-alpine",
        Cmd: []string{
            fmt.Sprintf("-Xmx%dM", cfg.RAMMB),
            fmt.Sprintf("-Xms%dM", cfg.RAMMB/2),
            "-jar", "server.jar",
            "nogui",
        },
        WorkingDir: "/server",
        ExposedPorts: nat.PortSet{
            "25565/tcp": {},
        },
        Labels: map[string]string{
            "demimine.managed": "true",
            "demimine.server_id": cfg.Name,
            "demimine.type": "server",
        },
    }

    hostConfig := &container.HostConfig{
        Binds: []string{
            fmt.Sprintf("%s:/server:rw", cfg.ServerPath),
            fmt.Sprintf("%s:/java:ro", cfg.JavaPath),
        },
        Resources: container.Resources{
            Memory: int64(cfg.RAMMB) * 1024 * 1024,
        },
        AutoRemove: false,
        NetworkMode: container.NetworkMode(cfg.NetworkName),
    }

    // Expose to host only if standalone (no proxy assigned)
    if cfg.HostPort > 0 {
        hostConfig.PortBindings = nat.PortMap{
            "25565/tcp": []nat.PortBinding{
                {HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.HostPort)},
            },
        }
    }

    // ... create container
}
```

### Proxy Container Template

```go
type ProxyContainerConfig struct {
    Name        string
    ProxyPath   string  // /proxies/{name}
    NetworkName string  // demimine_internal
    HostPort    int     // Port exposed to host (e.g., 25565)
}

func CreateProxyContainer(cfg ProxyContainerConfig) (*container.Container, error) {
    config := &container.Config{
        Image:      "itzg/mc-proxy",
        WorkingDir: "/server",
        ExposedPorts: nat.PortSet{
            "25577/tcp": {},
        },
        Labels: map[string]string{
            "demimine.managed": "true",
            "demimine.proxy_id": cfg.Name,
            "demimine.type": "proxy",
        },
    }

    hostConfig := &container.HostConfig{
        Binds: []string{
            fmt.Sprintf("%s:/server:rw", cfg.ProxyPath),
        },
        PortBindings: nat.PortMap{
            "25577/tcp": []nat.PortBinding{
                {HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", cfg.HostPort)},
            },
        },
        AutoRemove: false,
        NetworkMode: container.NetworkMode(cfg.NetworkName),
    }

    // ... create container
}
```

### Container Lifecycle

```
Create Server (API) → Pull base image → Create container → Start container
                                                                    ↓
Stop Server (API) → Send SIGTERM → Wait for graceful stop → Container exits
                                                              ↓
                                                    Mark status = stopped
                                                        
Crash detected → Non-zero exit → Capture logs → Create crash_log entry
                                                      ↓
                                          Mark status = crashed
```

**Crash Behavior**: When a server crashes, connected players are disconnected by their Minecraft client. This is expected client-side behavior and cannot be prevented. Players will see a "connection lost" screen. No auto-restart occurs - manual intervention is required to restart the server.

---

## Java Version Management

### Version Mapping

| Minecraft Version | Required Java |
|-------------------|---------------|
| 1.20.5+ | Java 21 |
| 1.18 – 1.20.4 | Java 17 |
| 1.17 | Java 16 (use 17) |
| 1.16.5 | Java 8 or 11 |
| ≤1.16.4 | Java 8 |

```go
func GetRequiredJavaVersion(mcVersion string) string {
    parts := strings.Split(mcVersion, ".")
    if len(parts) < 2 {
        return "21" // default
    }
    
    minor, _ := strconv.Atoi(parts[1])
    patch := 0
    if len(parts) > 2 {
        patch, _ = strconv.Atoi(strings.Split(parts[2], "-")[0])
    }
    
    switch {
    case minor > 20 || (minor == 20 && patch >= 5):
        return "21"
    case minor >= 18:
        return "17"
    case minor == 17:
        return "17" // Java 16 not widely available, 17 works
    case minor == 16 && patch == 5:
        return "17" // Prefer 17 for 1.16.5
    default:
        return "8"
    }
}
```

### Lazy Download

```go
func EnsureJavaVersion(version string) (string, error) {
    // Check if already installed
    if path, err := db.GetJavaPath(version); err == nil {
        return path, nil
    }
    
    // Download from Adoptium
    downloadURL := fmt.Sprintf(
        "https://api.adoptium.net/v3/binary/latest/%s/ga/linux/x64/jre/hotspot/normal/eclipse",
        version,
    )
    
    targetPath := filepath.Join(cfg.JavaDir, version)
    if err := downloadAndExtract(downloadURL, targetPath); err != nil {
        return "", err
    }
    
    // Record in database
    db.SaveJavaVersion(version, targetPath)
    
    return targetPath, nil
}
```

---

## Minecraft Server Support

### Supported Types

| Type | Version API | Notes |
|------|-------------|-------|
| Paper | `https://api.papermc.io/v2/projects/paper` | Direct jar download |
| Purpur | `https://api.purpurmc.org/v2/purpur` | Direct jar download |
| Fabric | `https://meta.fabricmc.net/v2/versions/game` | Requires installer (existing examples available) |
| NeoForge | `https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge` | Requires installer (existing examples available) |
| Forge | `https://files.minecraftforge.net/net/minecraftforge/forge/` | Requires installer (existing examples available) |

**Note**: Fabric, Forge, and NeoForge server download/installation code already exists as reference implementations. These will be adapted for DemiMine.

### Server Creation Flow

```
1. Validate input (name uniqueness, valid type/version)
2. Ensure Java version installed
3. Create directory: /servers/{name}
4. Download server jar (run installer for mod loaders)
5. Generate server.properties:
   - server-port=25565 (standard, internal network)
   - online-mode=false (proxy handles auth)
   - enable-rcon=false
   - etc.
6. Install FabricProxy-lite if Fabric (for Velocity forwarding)
7. Create eula.txt with eula=true (auto-accepted)
8. Save to database
9. Create Docker container (name: demimine-{server_name})
10. Register with proxy (if assigned) via velocity.toml update
```

### EULA Acceptance

The Minecraft EULA is automatically accepted on server creation. `eula.txt` is created with `eula=true`. This is a convenience feature for personal server management.

### server.properties Template

```properties
server-port=25565
online-mode=false
enable-rcon=false
enable-query=false
snooper-enabled=false
hardcore=false
allow-flight=true
spawn-protection=0
max-tick-time=60000
```

### Version Updates

**Policy**: Server version changes are handled manually by the admin, not via the API.

**Rationale**:
- Version updates typically require plugin updates
- World format changes may need conversion
- Allows testing on duplicated servers first

**Workflow**:
1. Duplicate the server (copies all files)
2. Manually update the new server's jar and plugins
3. Test the new server
4. Switch players to the new server when ready
5. Delete old server when no longer needed

### Container Naming Convention

| Container Type | Name Format | Example |
|----------------|-------------|---------|
| Manager | `demimine-manager` | demimine-manager |
| Proxy | `demimine-{proxy_name}` | demimine-main |
| Server | `demimine-{server_name}` | demimine-survival |

**Notes:**
- Names are lowercased and sanitized (spaces/special chars replaced with hyphens)
- Container names must be unique across all DemiMine-managed containers

### Server Reassignment

When a server's `proxy_id` is changed:

**Assigned to proxy (was standalone):**
1. Remove host port binding from container
2. Add container to internal Docker network
3. Update proxy's velocity.toml to include server
4. Clear `host_port` in database

**Reassigned to different proxy:**
1. Remove from old proxy's velocity.toml
2. Add to new proxy's velocity.toml

**Changed to standalone (was assigned to proxy):**
1. Remove from proxy's velocity.toml
2. Remove container from internal network (or keep for manager access)
3. Add host port binding
4. Update `host_port` in database

**Notes:**
- Container must be recreated for port binding changes (stop, delete, recreate, start)
- velocity.toml updates preserve manual edits to other sections

---

## Web UI Design

### Overview

The Web UI provides a visual, interactive interface for managing Minecraft servers and proxies. The main servers page features a zoomable/pannable canvas with a visual tree representation of server relationships.

### Main Servers Page (Canvas View)

**Visual Layout:**
- Infinite canvas with server tiles arranged in a 2D space
- Server tiles display: custom icon, name, status (Online/Offline), player count in parentheses
- Connection lines automatically drawn between proxies and their assigned servers
- Standalone servers (no proxy) have no connection lines and can be placed anywhere
- Top navigation bar with: DemiMine logo/brand, Servers (active), Settings
- Left sidebar with "Create+" button for adding servers/proxies
- Optional crash banner at bottom for unacknowledged crashes

**Example Tree Structure:**
```
[Proxy] ──────┬─── [Login] ───┬─── [Survival - Online (5)]
              │               ├─── [Hardcore - Online (2)]
              │               ├─── [Lobby - Online (12)]
              │               └─── [Creative - Offline (0)]
              │
[Dev 2 - Offline (0)]         [Dev - Offline (0)]
```

**Canvas Interactions:**

| Action | Input | Behavior |
|--------|-------|----------|
| Pan canvas | Left-click drag on background | Moves viewport around the 2D space |
| Move server | Shift + left-click drag on server tile | Moves server on invisible grid (snap-to-grid enabled) |
| Zoom | Mouse wheel or pinch gesture | Scales view from 50% to 200% |
| Select server | Click on server tile | Opens server detail page |

**Grid System:**
- Invisible grid for server placement (e.g., 100px increments)
- Shift+drag snaps server position to nearest grid point
- System prevents overlapping servers (minimum spacing enforced)
- Grid spacing should feel natural and allow organized layouts

**Connection Lines:**
- Automatically drawn from proxy to all assigned servers
- Lines update in real-time when servers are moved
- Style: straight lines or right-angle connectors (TBD during implementation)
- Visual distinction for proxy→server hierarchy

**Server Tile Details:**
- Custom icon (configurable in server settings, default placeholder if not set)
- Server name
- Status: "Online" or "Offline"
- Player count in parentheses: "(5)" or "(0)"
- Click anywhere on tile to open server detail page

**Proxy Tiles:**
- Proxies are visualized as tiles, same as servers
- Proxies have their own detail pages (logs, commands, files, settings)
- Connection lines originate from proxy tiles to their assigned servers

### Aesthetic & Styling

**Typography:**
- Primary font: Helvetica or system sans-serif equivalent (clean, readable)
- Avoid pixelated/blocky fonts for better readability

**Minecraft Theme:**
- Background textures: stone, wood, dirt block patterns (to be provided later)
- Textures scale appropriately with window size
- Textures should not overpower content - subtle, atmospheric background
- Accent colors inspired by Minecraft palette (greens, browns, blues)

**Responsive Design:**
- All elements scale proportionally with window size
- Works on any aspect ratio (16:9, 4:3, ultrawide, mobile)
- Canvas view adapts to viewport dimensions
- No fixed pixel widths for layout containers

**Color Scheme:**

```css
:root {
  --bg-primary: #0f0f0f;
  --bg-secondary: #1a1a1a;
  --bg-tertiary: #252525;
  --text-primary: #ffffff;
  --text-secondary: #a0a0a0;
  --accent: #3b82f6;       /* Blue */
  --accent-hover: #2563eb;
  --success: #22c55e;      /* Green - Online status */
  --warning: #eab308;      /* Yellow */
  --error: #ef4444;        /* Red - Offline/Crash */
  --border: #333333;
}
```

### Layout Structure

**Top Navigation Bar:**
- DemiMine logo/brand (left)
- Main nav items: Servers (active), Settings
- User menu/logout (right)

**Left Sidebar:**
- "Create+" button (opens modal to create server or proxy)
- Optional: Quick filters or view toggles

**Main Content Area:**
- Canvas fills remaining space
- No fixed sidebar list of servers (replaced by visual canvas)
- Crash banner appears at bottom when needed

### Key Components

#### Console Component

```svelte
<!-- Console.svelte -->
<script>
  export let serverId;
  export let logs = [];
  export let commandHistory = [];
  
  let command = "";
  let historyIndex = -1;
  
  function submitCommand() {
    if (!command.trim()) return;
    sendCommand(serverId, command);
    commandHistory = [command, ...commandHistory];
    command = "";
    historyIndex = -1;
  }
  
  function handleKeydown(e) {
    if (e.key === "ArrowUp") {
      e.preventDefault();
      if (historyIndex < commandHistory.length - 1) {
        historyIndex++;
        command = commandHistory[historyIndex];
      }
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      if (historyIndex > 0) {
        historyIndex--;
        command = commandHistory[historyIndex];
      } else {
        historyIndex = -1;
        command = "";
      }
    }
  }
</script>

<div class="console">
  <div class="log-output">
    {#each logs as log}
      <div class="log-line level-{log.level}">
        <span class="timestamp">{log.timestamp}</span>
        <span class="content">{log.line}</span>
      </div>
    {/each}
  </div>
  <div class="command-input">
    <input 
      type="text" 
      bind:value={command}
      on:keydown={handleKeydown}
      on:keydown:enter={submitCommand}
      placeholder="Enter command..."
    />
    <button on:click={submitCommand}>Send</button>
  </div>
</div>
```

#### File Tree Component

```svelte
<!-- FileTree.svelte -->
<script>
  export let serverId;
  export let currentPath = "/";
  export let entries = [];
  export let selectedFile = null;
  let isDragging = false;
  
  function navigate(path) {
    // Load directory contents
  }
  
  function openFile(entry) {
    if (!entry.is_dir) {
      selectedFile = entry;
      // Load file content
    }
  }
  
  function handleDragOver(e) {
    e.preventDefault();
    isDragging = true;
  }
  
  function handleDragLeave(e) {
    isDragging = false;
  }
  
  function handleDrop(e) {
    e.preventDefault();
    isDragging = false;
    const items = e.dataTransfer.items || e.dataTransfer.files;
    uploadFiles(items, currentPath);
  }
  
  function handleFilePicker() {
    const input = document.createElement("input");
    input.type = "file";
    input.multiple = true;
    input.webkitdirectory = false; // Allow files and folders
    input.onchange = (e) => uploadFiles(e.target.files, currentPath);
    input.click();
  }
</script>

<div 
  class="file-browser" 
  class:dragging={isDragging}
  on:dragover={handleDragOver}
  on:dragleave={handleDragLeave}
  on:drop={handleDrop}
>
  <div class="sidebar">
    <div class="path-bar">
      {#each currentPath.split("/").filter(Boolean) as segment}
        <button>{segment}</button>
        <span>/</span>
      {/each}
    </div>
    <ul class="file-list">
      {#each entries as entry}
        <li on:click={() => openFile(entry)} class:dir={entry.is_dir}>
          {entry.is_dir ? "📁" : "📄"} {entry.name}
        </li>
      {/each}
    </ul>
    <div class="actions">
      <button on:click={handleFilePicker}>Upload Files</button>
      <button>New Folder</button>
      <button>New File</button>
    </div>
    {#if isDragging}
      <div class="drop-overlay">
        Drop files here to upload
      </div>
    {/if}
  </div>
  <div class="content">
    {#if selectedFile}
      <CodeEditor {serverId} file={selectedFile} />
    {:else}
      <p>Select a file to view</p>
    {/if}
  </div>
</div>
```

### Pages

| Route | Description |
|-------|-------------|
| `/login` | Password-only login, redirect to setup if first launch |
| `/` | **Main servers page**: Canvas view with visual tree, pan/zoom, server tiles |
| `/servers/:id` | Server detail page with tabs for console, players, files, settings, backups |
| `/servers/:id/console` | Live console with command input, log history |
| `/servers/:id/players` | Online players with skins, kick/ban actions |
| `/servers/:id/files` | File browser with code editor, upload/download |
| `/servers/:id/settings` | Server configuration (RAM, port, auto-shutdown, icon upload, etc.) |
| `/servers/:id/backups` | Backup list, create/restore/delete actions |
| `/proxies/:id` | Proxy detail page with tabs |
| `/proxies/:id/console` | Proxy console |
| `/proxies/:id/servers` | Manage connected servers (assign/unassign) |
| `/proxies/:id/plugins` | Plugin management, upload/delete |
| `/proxies/:id/settings` | Proxy configuration, velocity.toml editor |
| `/admin/keys` | API key management |
| `/admin/java` | Java version management |
| `/admin/settings` | Manager settings |

**Notes:**
- No dedicated UI for whitelist/ops management - use console commands (`/whitelist add <player>`, `/op <player>`, etc.)
- This keeps the UI simple and avoids duplicating Minecraft's built-in permission systems
- The main page (`/`) IS the visual servers canvas, not a traditional dashboard

### Create Server Modal

When clicking the "Create+" button on the main canvas, a two-step modal appears:

**Step 1: Basic Info**
- Server name (text input)
- Server type (dropdown: Paper, Purpur, Fabric, NeoForge, Forge)
- Minecraft version (dropdown populated from `/api/versions/{type}`)

**Step 2: Configuration**
- RAM allocation (slider, 512MB - 16GB)
- Network configuration:
  - **Standalone** (default): Server accessible directly on a host port (defaults to 25565)
  - **Behind Proxy**: Server is internal-only, accessible only through a Velocity proxy

**Proxy Selection (Phase 4):**
When "Behind Proxy" is selected, a dropdown should appear showing available proxies fetched from `/api/proxies`. User must select which proxy to connect to. This is deferred to Phase 4 when proxy management is implemented.

**Default Behavior (Phase 3):**
Currently defaults to "Standalone" with port 25565. "Behind Proxy" option is shown but proxy selection is not yet implemented.

### Server Detail Page

When clicking a server tile from the main canvas, users are taken to the server detail page with tabbed navigation:

**Tab Structure:**
```
┌─────────────────────────────────────────────────────────────┐
│  ← Back to Servers     [Server Name] - Online (5)           │
├─────────────────────────────────────────────────────────────┤
│  [Console] [Files] [Settings] [Backups]                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│                    Tab Content Area                         │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

**Console Tab:**
- Live log output with timestamps
- Command input with history (arrow keys)
- Auto-scroll to bottom (pauses when user scrolls up)
- Log level color coding (INFO, WARN, ERROR)

**Files Tab:**
- File tree browser with directory navigation
- Code/text editor for config files
- Drag-and-drop upload support
- Download, rename, delete actions
- Breadcrumb path navigation

**Settings Tab:**
- Server icon upload
- RAM allocation slider
- Auto-shutdown timer configuration
- Backup interval settings
- Scheduled start/stop times
- Domain configuration (for forced hosts)
- Proxy assignment dropdown
- Server JAR update button

**Backups Tab:**
- List of existing backups with size/date
- Create new backup button
- Restore button (with confirmation)
- Delete backup button

**Performance Monitoring:**
- CPU usage graph (real-time)
- Memory usage graph (real-time)
- Uptime display
- Player count history

### Visual Tree Implementation

**Canvas Rendering:**
- Use HTML5 Canvas or SVG for server tiles and connection lines
- Server positions stored in database or localStorage
- Initial load: render all servers at saved positions
- Connection lines rendered on top of background, behind server tiles

**Position Storage:**
```sql
-- Add to servers table
ALTER TABLE servers ADD COLUMN canvas_x INTEGER DEFAULT 0;
ALTER TABLE servers ADD COLUMN canvas_y INTEGER DEFAULT 0;
```

**Connection Line Algorithm:**
```
For each proxy:
  Get all servers assigned to proxy
  For each server:
    Draw line from (proxy.x, proxy.y) to (server.x, server.y)
    Style: semi-transparent, smooth curves or straight lines
```

**Grid Snapping:**
```javascript
const GRID_SIZE = 100; // pixels

function snapToGrid(x, y) {
  return {
    x: Math.round(x / GRID_SIZE) * GRID_SIZE,
    y: Math.round(y / GRID_SIZE) * GRID_SIZE
  };
}
```

**Overlap Prevention:**
- Minimum distance between server centers: 150px
- On server move, check collision with all other servers
- If collision detected, snap to nearest valid grid position
- Visual feedback during drag (ghost outline at snap position)

**Zoom Implementation:**
- Transform scale on canvas container
- Range: 0.5 (50%) to 2.0 (200%)
- Mouse wheel: adjust scale in 0.1 increments
- Preserve server positions (scale relative to canvas center)

**Pan Implementation:**
- Track mouse position on mousedown
- On mousemove (while dragging background): adjust canvas offset
- Cursor changes to "grab" on hover, "grabbing" while dragging

### Implementation Notes

**Frontend Library Recommendations:**
- **Canvas rendering**: Consider using Konva.js, Fabric.js, or raw HTML5 Canvas API
- **State management**: Svelte stores for server positions, zoom level, pan offset
- **WebSocket**: Real-time updates for server status changes, player counts
- **Persistence**: Save canvas positions to database via API on drag end

**Key Implementation Tasks:**
1. Create main canvas component with pan/zoom functionality
2. Implement server tile component with icon, name, status, player count
3. Add grid snapping logic with overlap detection
4. Render connection lines between proxies and servers
5. Handle server click navigation to detail page
6. Persist canvas positions on drag end
7. Add texture backgrounds (pluggable, added later)
8. Implement responsive layout for various screen sizes

**Testing Considerations:**
- Test with 1-50 servers to ensure performance
- Test zoom/pan on different screen sizes
- Test overlap prevention edge cases
- Test connection line rendering with complex hierarchies

---

## Velocity Proxy Integration

**Reference Implementations:**
- Auth plugin: `/home/strasburg/Documents/clones/loginPassword`
- Dynamic server plugin: `/home/strasburg/Documents/IntelliJ Projects/PufferPanel AutoStartStop`

These existing implementations will be adapted for DemiMine.

### Auth Plugin

**Purpose**: Intercept player joins, require password before allowing access to backend servers.

**Flow**:
1. Player connects to proxy
2. Plugin intercepts `LoginEvent`
3. Player sent to limbo server (virtual, no actual server)
4. Plugin listens for chat messages
5. Player types password in chat
6. If correct: grant `demimine.authenticated` permission, send to nexus
7. If incorrect: increment attempts, disconnect after 3

**Configuration** (`plugins/demi-auth/config.toml`):
```toml
password = "hashed_bcrypt_password"
nexus_server = "nexus"
max_attempts = 3
kick_message = "Too many failed attempts"
success_message = "§aAuthenticated! Sending you to the hub..."
prompt_message = "§ePlease type the password in chat to continue."
```

### Dynamic Plugin

**Purpose**: Start stopped servers on demand, shutdown idle servers.

**Flow (Server Start)**:
1. Player attempts to connect to stopped server
2. Plugin intercepts `ServerPreConnectEvent`
3. Check if player has `demimine.authenticated` permission
4. If not authenticated, deny with message
5. Call Manager API: `POST /api/servers/:name/start`
6. If error (insufficient RAM):
   - Check for servers with `auto_shutdown` active
   - Call `POST /api/servers/:id/stop` on shortest-timer server
   - Retry start
   - If still fails, deny connection with error message
7. Show "Starting server..." message
8. Poll server status every 1 second
9. On `running`, allow connection

**Flow (Server Stop)**:
1. Player leaves server
2. Check if auto-shutdown enabled for that server
3. Start 15-minute timer
4. If player rejoins, cancel timer
5. On timer expiry, call Manager API: `POST /api/servers/:id/stop`

**Configuration** (`plugins/demi-dynamic/config.toml`):
```toml
manager_url = "http://host.docker.internal:8080"
api_key = "from-manager-api-keys-page"
check_interval_seconds = 1
start_timeout_seconds = 120
```

### velocity.toml Updates

The manager automatically updates the proxy's `velocity.toml` whenever servers are assigned to or removed from a proxy.

**On server assigned to proxy:**
```toml
[servers]
{server_name} = "{container_name}:25565"

[forced-hosts]
"{domain}" = [           # Only if domain is configured
  "{server_name}"
]
```

**On server removed from proxy:**
- Remove entry from `[servers]` section
- Remove any associated forced-host entries

**Notes:**
- Container names are used for internal DNS resolution on the Docker network
- Managers should validate TOML syntax after updates
- Users can manually edit velocity.toml for advanced configs (manager preserves manual changes)

---

## Backup System

### Scheduler

```go
type BackupScheduler struct {
    db       *db.DB
    backupDir string
    ticker   *time.Ticker
}

func (s *BackupScheduler) Start() {
    s.ticker = time.NewTicker(1 * time.Hour)
    go func() {
        for range s.ticker.C {
            s.checkBackups()
        }
    }()
}

func (s *BackupScheduler) checkBackups() {
    servers, _ := s.db.GetServersWithBackupEnabled()
    for _, server := range servers {
        if s.shouldBackup(server) {
            go s.createBackup(server)
        }
    }
}

func (s *BackupScheduler) shouldBackup(server Server) bool {
    lastBackup, _ := s.db.GetLastBackup(server.ID)
    if lastBackup == nil {
        return true
    }
    hoursSince := time.Since(lastBackup.CreatedAt).Hours()
    return hoursSince >= float64(server.BackupIntervalDays*24)
}
```

### Backup Creation

```go
func CreateBackup(server Server) error {
    timestamp := time.Now().Format("20060102")
    backupPath := filepath.Join(
        backupDir,
        fmt.Sprintf("%s_%s", server.Name, timestamp),
    )
    
    // Stop server if running (optional, configurable)
    wasRunning := server.Status == "running"
    if wasRunning {
        StopServer(server.ID)
        defer StartServer(server.ID)
    }
    
    // Copy entire server directory
    if err := copyDir(server.Path, backupPath); err != nil {
        return err
    }
    
    // Calculate size
    size := dirSize(backupPath)
    
    // Record in database
    db.CreateBackup(server.ID, backupPath, size)
    
    // Prune old backups (keep latest 5)
    pruneBackups(server.ID, 5)
    
    return nil
}
```

### Backup Restoration

```go
func RestoreBackup(serverID int, backupID int) error {
    server, _ := db.GetServer(serverID)
    backup, _ := db.GetBackup(backupID)
    
    // Stop server if running
    if server.Status == "running" {
        StopServer(serverID)
    }
    
    // Delete current server files (except logs)
    keepPatterns := []string{"logs/*"}
    clearDir(server.Path, keepPatterns)
    
    // Copy backup to server directory
    if err := copyDir(backup.Path, server.Path); err != nil {
        return err
    }
    
    return nil
}
```

---

## Security

### Password Storage

```go
func HashPassword(password string) (string, error) {
    bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    return string(bytes), err
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

### Session Management

```go
type Claims struct {
    UserID int `json:"user_id"`
    jwt.RegisteredClaims
}

func GenerateToken(userID int) (string, error) {
    claims := Claims{
        UserID: userID,
        RegisteredClaims: jwt.RegisteredClaims{
            ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
            IssuedAt:  jwt.NewNumericDate(time.Now()),
        },
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString(jwtSecret)
}

func ValidateToken(tokenString string) (*Claims, error) {
    token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
        return jwtSecret, nil
    })
    if err != nil {
        return nil, err
    }
    if claims, ok := token.Claims.(*Claims); ok && token.Valid {
        return claims, nil
    }
    return nil, errors.New("invalid token")
}
```

### API Key Authentication

```go
func GenerateAPIKey() string {
    bytes := make([]byte, 32)
    rand.Read(bytes)
    return base64.URLEncoding.EncodeToString(bytes)
}

func ValidateAPIKey(key string) bool {
    hash := sha256.Sum256([]byte(key))
    storedHash, err := db.GetAPIKeyHash(hash[:])
    if err != nil {
        return false
    }
    // Constant-time comparison
    return subtle.ConstantTimeCompare(hash[:], storedHash) == 1
}
```

### Middleware

```go
func AuthMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Check session cookie
        cookie, err := r.Cookie("session")
        if err == nil {
            claims, err := ValidateToken(cookie.Value)
            if err == nil {
                ctx := context.WithValue(r.Context(), "user_id", claims.UserID)
                next.ServeHTTP(w, r.WithContext(ctx))
                return
            }
        }
        
        // Check Authorization header (for API keys)
        authHeader := r.Header.Get("Authorization")
        if strings.HasPrefix(authHeader, "Bearer ") {
            key := strings.TrimPrefix(authHeader, "Bearer ")
            if ValidateAPIKey(key) {
                next.ServeHTTP(w, r)
                return
            }
        }
        
        http.Error(w, `{"error": "unauthorized"}`, http.StatusUnauthorized)
    })
}
```

### Rate Limiting

```go
type RateLimiter struct {
    attempts map[string]*Attempt
    mu       sync.Mutex
}

type Attempt struct {
    Count     int
    FirstSeen time.Time
}

func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        ip := r.RemoteAddr
        rl.mu.Lock()
        defer rl.mu.Unlock()
        
        attempt, exists := rl.attempts[ip]
        if !exists || time.Since(attempt.FirstSeen) > 15*time.Minute {
            rl.attempts[ip] = &Attempt{Count: 1, FirstSeen: time.Now()}
        } else {
            attempt.Count++
            if attempt.Count > 10 {
                http.Error(w, "too many requests", http.StatusTooManyRequests)
                return
            }
        }
        
        next.ServeHTTP(w, r)
    })
}
```

---

## Implementation Phases

### Phase 1: Foundation (Week 1)

**Days 1-2: Project Setup**
- [ ] Initialize Go module
- [ ] Set up project structure
- [ ] Create Dockerfile and docker-compose.yml
- [ ] Database migrations and connection
- [ ] Basic config loading

**Days 3-4: Core API**
- [ ] Router setup with Chi
- [ ] Auth endpoints (setup, login, logout)
- [ ] Session middleware
- [ ] Basic server CRUD endpoints

**Day 5: Docker Integration**
- [ ] Docker SDK integration
- [ ] Container creation template
- [ ] Start/stop container logic
- [ ] Port allocation system

### Phase 2: Server Management (Week 2)

**Days 1-2: Server Creation**
- [ ] Version fetching for all server types
- [ ] Server jar download logic
- [ ] Java version management
- [ ] server.properties generation
- [ ] FabricProxy installation

**Days 3-4: Server Operations**
- [ ] Console log streaming
- [ ] Command execution
- [ ] Player tracking
- [ ] Resource monitoring

**Day 5: File Management**
- [ ] File browser API
- [ ] File upload/download
- [ ] File editing

### Phase 3: Web UI (Weeks 3-4)

**Days 1-3: Core UI**
- [ ] SvelteKit setup with Tailwind
- [ ] Layout and navigation
- [ ] Login page
- [ ] Dashboard

**Days 4-6: Server Pages**
- [ ] Server creation wizard
- [ ] Console component
- [ ] Player list
- [ ] File browser with editor
- [ ] Settings page

**Days 7-8: Admin & Polish**
- [ ] Proxy management
- [ ] API keys page
- [ ] Java management
- [ ] Crash banners
- [ ] WebSocket real-time updates

### Phase 4: Velocity Plugins (Week 5)

**Days 1-3: Auth Plugin**
- [ ] Plugin scaffold
- [ ] Limbo server implementation
- [ ] Password authentication
- [ ] Permission handling

**Days 4-5: Dynamic Plugin**
- [ ] API client for manager
- [ ] Server start on connect
- [ ] Idle shutdown timer
- [ ] RAM management logic

### Phase 5: Backup System (Week 5-6)

**Days 1-2: Backup Logic**
- [ ] Backup scheduler
- [ ] Backup creation
- [ ] Backup restoration
- [ ] Auto-pruning

**Days 3-4: UI Integration**
- [ ] Backup list page
- [ ] Create/restore UI
- [ ] Backup settings

### Phase 6: Polish & Testing (Week 6)

- [ ] Error handling review
- [ ] Graceful shutdown
- [ ] Documentation
- [ ] Integration testing
- [ ] Performance optimization

---

## Appendix

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DEMIMINE_PORT` | `8080` | Web UI and API port |
| `DEMIMINE_DATA_DIR` | `/data` | SQLite and config location |
| `DEMIMINE_SERVERS_DIR` | `/servers` | Backend server files |
| `DEMIMINE_PROXIES_DIR` | `/proxies` | Proxy files (velocity.toml, plugins) |
| `DEMIMINE_BACKUPS_DIR` | `/backups` | Backup storage |
| `DEMIMINE_JAVA_DIR` | `/java` | Java runtime storage |
| `DEMIMINE_NETWORK` | `demimine_internal` | Docker network for servers/proxies |
| `DEMIMINE_MAX_RAM_MB` | `0` | Max allocatable RAM (0 = unlimited) |
| `DEMIMINE_JWT_SECRET` | (random) | JWT signing secret |

### Default Ports

| Service | Port | Notes |
|---------|------|-------|
| Web UI / API | 8080 | Manager container |
| Velocity Proxy | 25565 | Configurable per proxy (UI defaults to 25565) |
| Proxied Servers | 25565 | Internal only, not exposed to host |
| Standalone Servers | User-assigned | Exposed to host for direct connections |

### File Locations

| File/Directory | Location |
|----------------|----------|
| SQLite Database | `./data/demimine.db` |
| Server Files | `./servers/{server_name}/` |
| Proxy Files | `./proxies/{proxy_name}/` |
| Backups | `./backups/{server_name}_{date}/` |
| Java Runtimes | `./java/{version}/` |
| Manager Logs | `./data/manager.log` |

### Useful Commands

```bash
# Start DemiMine
docker compose up -d

# View manager logs
docker compose logs -f manager

# Update DemiMine
docker compose pull && docker compose up -d

# Access SQLite database
sqlite3 ./data/demimine.db

# Manual backup
curl -X POST http://localhost:8080/api/servers/1/backups \
  -H "Authorization: Bearer $TOKEN"

# List running containers
docker ps --filter label=demimine.managed=true
```

### References

| Resource | Path | Usage |
|----------|------|-------|
| Auth Plugin Example | `/home/strasburg/Documents/clones/loginPassword` | Reference for Velocity auth plugin |
| Dynamic Server Plugin | `/home/strasburg/Documents/IntelliJ Projects/PufferPanel AutoStartStop` | Reference for auto-start/stop logic |
| PufferPanel | `/home/strasburg/Documents/clones/pufferpanel` | Reference only - Docker integration ideas. Do not copy code. Consult only when prompted or for second opinions. |
