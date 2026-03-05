# DemiMine

A fully-isolated Minecraft proxy and server manager with a web UI, deployed via Docker Compose.

## Quick Summary

DemiMine manages multiple Minecraft backend servers (Paper, Purpur, Fabric, NeoForge, Forge) behind one or more Velocity proxies. Servers are dynamically started/stopped based on player activity, with lazy-loaded Java runtimes and an integrated backup system.

**Key Features:**
- Web UI accessible over LAN for headless server management
- Dynamic server start/stop (15-min idle shutdown)
- Automatic Java version management (lazy-loaded from Adoptium)
- Multiple Velocity proxies supported (each in own container)
- Custom auth and dynamic server plugins for Velocity
- Full file browser and console access via Web UI
- Drag-and-drop file upload with multi-file/folder support
- Backup system with restoration
- Backend servers on internal Docker network (not exposed to host)
- Standalone server support (no proxy required)
- Server duplication for safe version updates
- Automatic velocity.toml sync when servers assigned/removed
- Server JAR update action (maintains MC version)
- Port conflict warnings with override option
- EULA auto-accepted on server creation

**Tech Stack:** Go (backend) + Svelte (frontend) + SQLite + Docker

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
│              │  Velocity   │ ← Exposed to host (:25565)       │
│              │  Proxy(s)   │                                   │
│              └─────────────┘                                   │
└────────────────────────────────────────────────────────────────┘
                      ▲
                      │ Docker Socket
┌─────────────────────┴────────────────────────────────────────┐
│  Manager Container (Go API + Svelte UI + SQLite)             │
└──────────────────────────────────────────────────────────────┘
```

## Implementation

See [design.md](./design.md) for the full implementation plan including:
- Complete API specification
- Database schema
- Docker integration details
- Web UI design
- Velocity plugin specifications
- Backup system logic
- Security implementation

## Getting Started (Planned)

```bash
docker compose up -d
# Open http://localhost:8080
# Set admin username and password on first launch
# Create a proxy, then create servers assigned to it
```

## Project Status

**Planning complete.** Implementation not yet started.

See [design.md](./design.md) for the comprehensive implementation plan.
