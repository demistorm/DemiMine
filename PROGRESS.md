# DemiMine Development Progress

## Overall Status: Production-Ready
Phases 1-6 are complete and fully functional. DemiMine now provides comprehensive server management, proxy integration, player tracking, auto-shutdown, backup system, and plugin management capabilities.

## Phase 1: Foundation ✅ COMPLETE
- Go backend with Chi router and SQLite database
- Auth endpoints (setup, login, logout, status) with JWT
- Server CRUD endpoints with port conflict detection
- Rate limiting, CORS, and logging middleware
- Docker Compose deployment
- API key management (create, list, delete) for plugin authentication
- Player tracking and management (join/leave events, per-server player counts)

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

## Database Schema
- SQLite with WAL mode and automatic migrations (versioned)
- Tables: proxies, servers, players, api_keys, settings, backups, crash_logs, command_history, admin_auth, installed_plugins
- Player tracking: join/leave events, current server per player
- API key management: secure keys for plugin authentication

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
- ✅ Server detail pages with tabs (console, files, settings)
- ✅ File browser component with full API integration (list, read, write, delete, download, rename, drag-drop upload)
- ✅ Settings page with RAM allocation, auto-shutdown, backup configuration
- ✅ Create Server modal with version dropdown (fetches versions from API)
- ✅ Backend PATCH endpoint for server updates
- ✅ Icon upload for servers and proxies (with preview)
- ✅ WebSocket integration for real-time updates

## Phase 4: Proxy System ✅ COMPLETE
- ✅ Velocity proxy container management
- ✅ Proxy CRUD endpoints (create, read, update, delete)
- ✅ Proxy console and file access
- ✅ Proxy icon upload
- ✅ Automatic velocity.toml sync when servers assigned/removed
- ✅ MiniMOTD integration for server list customization
  - Auto-install on new proxy creation
  - Custom MOTD configuration via web UI
  - Per-server MOTD customization
- ✅ Plugin management for proxies
  - Modrinth API integration for searching and installing plugins
  - Plugin version checking and updates
  - Global MC version filtering for plugin compatibility
- ✅ DemiAuth plugin (chat-based authentication, no `/login` command)
  - Permission node: `demimine.authenticated`
  - 120-second timeout, 3-attempt disconnect limit
  - Auto-configured for NanoLimbo servers named with "auth" or "login"
- ✅ DemiDynamic plugin v1.0.3 (auto start/stop servers based on player activity)
  - Server start on player connection attempt
  - Player join/leave tracking and reporting
  - Auto-shutdown timer (configurable, 15-min default)
  - Queue system with countdown for starting servers
  - Server exclusion list for always-on servers (login, queue)
  - Race condition fixes and proper server transfer detection
  - Cancel timer when players rejoin (with INFO logging)
  - Hub server support with direct connection fallback
  - Configuration with manager_url, api_key, queue_server, hub_server
- ✅ NanoLimbo server type support (BoomEaro fork)
- ✅ LuckPerms auto-installation option for proxies
- ✅ Proxy creation UI with DemiAuth/DemiDynamic/LuckPerms checkboxes
- ✅ Server creation UI with NanoLimbo option

## Phase 5: Advanced Features ✅ COMPLETE
- ✅ API key management
  - Create, list, and delete API keys
  - Keys used by DemiDynamic plugin for server control
  - Rate limiting (500 req/15min) with path exclusions for health/ws/resources
- ✅ Player management and tracking
  - Player join/leave events reported to manager
  - Player count tracking per server
  - Support for excluded servers (always-on)
- ✅ Auto-shutdown timer
  - Per-server configuration (minutes)
  - Idle server shutdown when players leave
  - Warning message 1 minute before shutdown
  - Canceled when players rejoin
- ✅ Port-sharing confirmation for standalone servers
- ✅ JAR update system (PaperMC Fill API v3)
  - Check for available updates (server + proxy)
  - Download and apply updates (server must be stopped)
  - Build number and hash tracking
- ✅ OOM (Out of Memory) fix for container handling
- ✅ Scheduled start/stop with configurable times
- ✅ Spark profiler integration (start/stop profiling from UI)

## Phase 6: Backup System ✅ COMPLETE
- ✅ Full-system backup (data + servers, optional java)
  - Stops all servers/proxies before backup, restarts after
  - Configurable backup time and interval (daily default)
  - Configurable retention policy
  - WebSocket progress notifications
- ✅ Backup restoration
  - Restore from disk or upload
  - Stops all servers/proxies, restores data, restarts
- ✅ Backup management UI (settings page)
  - List, download, restore, delete backups

## Production Hardening ✅ COMPLETE
- ✅ Docker HEALTHCHECK with real DB + Docker socket verification
- ✅ Config validation at startup (port, host servers dir, network, max RAM)
- ✅ Rate limiter with IP spoofing fix (leftmost X-Forwarded-For)
- ✅ Path exclusions from rate limiting (/health, /api/ws, /api/system/resources)
- ✅ Versioned database migrations with error logging
- ✅ DB connection lifetime (5min) for WAL checkpointing
- ✅ Docker `destroy` event handling (status sync)
- ✅ Consistent JSON error responses with internal error logging
- ✅ Configurable timezone (TZ env var, falls back to America/Chicago)
- ✅ Graceful shutdown with SIGTERM handling
- ✅ JWT validation fix (algorithm check in WebSocket)
- ✅ Plugin endpoints behind auth middleware
- ✅ RCON password required at startup
- ✅ GitHub Actions CI/CD for Docker image builds
- ✅ Logging middleware includes request IDs

## Recent Bug Fixes & Improvements
- Fixed case sensitivity issues in server/proxy lookups (prioritize exact matches)
- Fixed log/console streaming container name resolution
- Added graceful handling of missing containers in stop handlers
- Implemented proper wait for container full stop before restart (both proxies and servers)
- Fixed MiniMOTD configuration file paths and update handling
- Removed game version requirement for proxy plugins (compatibility improvement)
- Fixed JAR update version display and build number tracking
- Fixed DemiDynamic server queue connection issues
- Fixed DemiDynamic auto-stop and server transfer detection
- Reduced DemiDynamic JAR size through shadowJar optimization
- Added .svelte-kit to .gitignore (exclude build artifacts)
- Updated AGENTS.md and design.md with Fill API migration details

## Statistics
- **Backend**: 10,800+ lines of Go code
- **Frontend**: SvelteKit 2.0, TypeScript 5.0, Tailwind 3.4
- **API Endpoints**: 10+ handlers covering all features
- **Database**: 10 tables with versioned automatic migrations
- **Docker**: Multi-stage build with Alpine runtime and HEALTHCHECK
- **Tests**: 770+ lines of unit tests (mc + backup)
- **Plugins**: DemiAuth (auth), DemiDynamic (auto-start/stop), MiniMOTD (MOTD)
- **Git**: 45+ commits, actively developed
