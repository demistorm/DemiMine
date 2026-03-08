# DemiMine Development Progress

## Phase 1: Foundation ✅ COMPLETE
- Go backend with Chi router and SQLite database
- Auth endpoints (setup, login, logout, status) with JWT
- Server CRUD endpoints with port conflict detection
- Rate limiting, CORS, and logging middleware
- Docker Compose deployment

## Phase 2: Server Management ✅ COMPLETE
- Version fetching (Paper/Purpur/Fabric/NeoForge/Forge)
- Server jar download logic
  - **Paper/Purpur**: Direct download from API
  - **Fabric**: Direct download from Fabric meta API (pre-built server jar)
  - **Forge/NeoForge**: Download installer, run with `--installServer`, creates `run.sh` and libraries
- server.properties generation
- EULA auto-acceptance
- Docker container creation with Java image selection
- Container start/stop/restart endpoints
- Console log retrieval endpoint
- Command execution endpoint
- File browser API (list, read, write, delete, download, rename, upload)
- Added HostServersDir config for proper Docker bind mounts
- Unit tests for mc package (versions, properties, java version detection)
- Fixed Java version detection for edge cases

## Remaining: Fabric/Forge/NeoForge Notes
The Forge/NeoForge installers are executed during server creation, which can take 1-2 minutes on first run. The installer downloads Minecraft server files and required libraries, then creates a `run.sh` script for launching the server.

## Phase 3: Web UI ✅ COMPLETE
- ✅ SvelteKit project setup with TypeScript and Tailwind CSS
- ✅ Basic component structure (API client, stores, Console, ServerTile, Login page)
- ✅ Main canvas page with pan/zoom infrastructure
- ✅ Frontend builds successfully
- ✅ Dockerfile updated with multi-stage build (Go backend + Svelte frontend)
- ✅ Backend router configured to serve static files from /app/webui/build
- ✅ API endpoints aligned (fixed /api/auth/* endpoints)
- ✅ Docker container builds and runs successfully
- ✅ Server detail pages with tabs (console, files, settings, backups)
- ✅ File browser component with full API integration (list, read, write, delete, download, rename, drag-drop upload)
- ✅ Settings page with RAM allocation, auto-shutdown, backup interval
- ✅ Create Server modal with version dropdown (fetches versions from API)
- ✅ Backend PATCH endpoint for server updates
- ⏳ WebSocket integration for real-time updates (deferred)
- ⏳ Icon upload (deferred)

## Phase 4: Proxy System (PLANNED)
- Velocity proxy container management
- Custom auth plugin for Velocity
- Dynamic server registration plugin for Velocity
- Proxy configuration sync

## Phase 5: Backup System (PLANNED)
- Backup creation (scheduled and manual)
- Backup restoration
- Backup retention policy

## Phase 6: Advanced Features (PLANNED)
- Player activity tracking
- Auto-shutdown timer
- Scheduled start/stop
- Server duplication feature
- Server JAR update action
