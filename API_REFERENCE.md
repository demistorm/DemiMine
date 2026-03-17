# DemiDynamic API Reference

## Overview

DemiDynamic plugin communicates with the DemiMine Manager API via the following endpoints. All endpoints require API key authentication (except player list which uses JWT).

## Authentication

All API requests must include an `Authorization` header with a Bearer token:

```
Authorization: Bearer your-api-key-here
```

API keys can be generated from the Manager web UI at `/api-keys`.

## Endpoints

### Server Management

#### Start a Server

```http
POST /api/servers/{name}/start
```

**Parameters:**
- `name` (path) - Server name to start

**Response:**
```json
{
  "success": true,
  "message": "Server survival started"
}
```

**Error Response:**
```json
{
  "error": "failed to start server: insufficient RAM"
}
```

---

#### Stop a Server by Name

```http
POST /api/servers/{name}/stop-by-name
```

**Parameters:**
- `name` (path) - Server name to stop

**Response:**
```json
{
  "server_id": "123"
}
```

**Used by:** DemiDynamic `/cancel` command to cancel server start

---

#### Get Server Status

```http
GET /api/servers/{name}/status
```

**Parameters:**
- `name` (path) - Server name

**Response:**
```json
{
  "name": "survival",
  "status": "running",
  "auto_shutdown_minutes": 15,
  "player_count": 2
}
```

**Status values:** `stopped`, `starting`, `running`

**Used by:**
- DemiDynamic to poll server status while waiting for it to start
- DemiDynamic to check if auto-shutdown is enabled

---

#### Get Auto-Shutdown Servers

```http
GET /api/servers/auto-shutdown
```

**Response:**
```json
[
  {
    "id": 123,
    "name": "survival",
    "ram_mb": 4096
  },
  {
    "id": 124,
    "name": "creative",
    "ram_mb": 2048
  }
]
```

**Note:** Returns only running servers with `auto_shutdown_minutes > 0`, sorted by timer (shortest first).

**Used by:** DemiDynamic RAM management - to find a server to stop when start fails due to insufficient RAM

---

### Player Tracking

#### Report Player Join

```http
POST /api/players/join
```

**Request Body:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "name": "PlayerName",
  "server_name": "survival"
}
```

**Response:**
```json
{
  "status": "ok"
}
```

**Used by:** DemiDynamic when a player successfully connects to a server

---

#### Report Player Leave

```http
POST /api/players/leave
```

**Request Body:**
```json
{
  "uuid": "550e8400-e29b-41d4-a716-446655440000",
  "server_name": "survival"
}
```

**Response:**
```json
{
  "status": "ok"
}
```

**Used by:** DemiDynamic when a player disconnects from a server

---

#### Get Server Players (Web UI)

```http
GET /api/servers/{id}/players
```

**Authentication:** JWT (session cookie or Bearer token)

**Parameters:**
- `id` (path) - Server ID

**Response:**
```json
[
  {
    "uuid": "550e8400-e29b-41d4-a716-446655440000",
    "name": "PlayerName",
    "joined_at": "2026-03-17T20:24:00Z"
  }
]
```

**Used by:** DemiMine Web UI to display players on server detail pages

---

## Plugin Configuration

The DemiDynamic plugin uses the following configuration values (in `plugins/DemiDynamic/config.toml`):

```toml
manager_url = "http://host.docker.internal:8080"
api_key = "your-api-key-here"
check_interval_seconds = 1
start_timeout_seconds = 120
auto_stop_timeout_minutes = 15
```

## Flow Examples

### Server Start Flow

```
1. Player connects → ServerPreConnectEvent
2. Check demimine.authenticated permission
3. GET /api/servers/{name}/status → server is "stopped"
4. POST /api/servers/{name}/start
5. If error (RAM full):
   a. GET /api/servers/auto-shutdown
   b. POST /api/servers/{id}/stop (stop server with shortest timer)
   c. Retry POST /api/servers/{name}/start
6. Queue player (send to nexus or keep in hub)
7. Poll GET /api/servers/{name}/status every 1s
8. When status = "running":
   a. Allow connection
   b. POST /api/players/join
```

### Server Stop Flow

```
1. Player disconnects → DisconnectEvent
2. Remove from server player set
3. POST /api/players/leave
4. GET /api/servers/{name}/status → check auto_shutdown_minutes
5. If auto_shutdown_minutes > 0 AND server empty:
   a. Start 15-minute timer
   b. On timeout → POST /api/servers/{name}/stop-by-name
```

### Cancel Command Flow

```
1. Player runs /cancel
2. Check player's current server
3. Remove from queue
4. If in queue server:
   a. Disconnect player with "Player canceled the queue!"
5. If in hub server:
   a. Send "Canceled queue for {server}!"
   b. Keep player in hub
6. If queue now empty:
   a. POST /api/servers/{name}/stop-by-name (cancel server start)
```

## Error Handling

### Common Error Codes

- `400` - Bad request (invalid parameters)
- `401` - Unauthorized (invalid API key)
- `404` - Not found (server doesn't exist)
- `500` - Internal server error (database or Docker error)

### Error Response Format

```json
{
  "error": "descriptive error message"
}
```

### Plugin Error Handling

When the plugin encounters errors:
- **Start failure:** Tries to stop an auto-shutdown server and retries
- **Still failing after retry:** Denies connection with message
- **Timeout:** Disconnects player with "Server failed to start. Please try again later."
- **API unavailable:** Logs error, attempts to continue with fallback logic
