# DemiDynamic Implementation Summary

## Completed Implementation

### Phase 1: Backend API Endpoints

**File:** `manager/internal/api/handlers/players.go`

New endpoints added:

1. **POST /api/players/join** - API Key authentication required
   - Request: `{ "uuid": "...", "name": "...", "server_name": "..." }`
   - Records player joined a server in the database
   - Returns 200 OK or 404 if server not found

2. **POST /api/players/leave** - API Key authentication required
   - Request: `{ "uuid": "...", "server_name": "..." }`
   - Removes player from server in the database
   - Returns 200 OK or 404 if server not found

3. **GET /api/servers/{id}/players** - JWT authentication required
   - Returns list of players on a server
   - Response: `[{ "uuid": "...", "name": "...", "joined_at": "..." }]`

4. **POST /api/servers/{name}/stop-by-name** - API Key authentication required
   - Returns server ID for a given server name
   - Used by DemiDynamic plugin to stop servers by name
   - Returns 200 OK with `{ "server_id": "123" }`

5. **GET /api/servers/auto-shutdown** - API Key authentication required
   - Returns list of running servers with `auto_shutdown_minutes > 0`
   - Sorted by auto_shutdown_minutes ascending (shortest timer first)
   - Response: `[{ "id": 123, "name": "survival", "ram_mb": 4096 }]`
   - Used for RAM management: stop a server to free resources

6. **GET /api/servers/{name}/status** - API Key authentication required
   - Returns server status including auto-shutdown and player count info
   - Response: `{ "name": "survival", "status": "running", "auto_shutdown_minutes": 15, "player_count": 2 }`
   - Used for status polling and auto-shutdown eligibility check

**Router Updates:**
- Added `playerHandler` to `router.go`
- Added routes for player join/leave, auto-shutdown servers list, server status, stop-by-name

### Phase 2: DemiDynamic Plugin

**Directory Structure:**
```
DemiDynamic/
├── build.gradle
├── gradle.properties
├── settings.gradle
├── README.md
└── src/main/
    ├── java/net/demimine/dynamic/
    │   ├── DemiDynamic.java (main plugin class)
    │   ├── Config.java (config handling)
    │   ├── ApiClient.java (HTTP client for Manager API)
    │   ├── QueueManager.java (queue management)
    │   ├── AutoStopManager.java (auto-stop timers)
    │   ├── handlers/
    │   │   ├── ServerPreConnectHandler.java
    │   │   ├── DisconnectHandler.java
    │   │   └── ServerConnectedHandler.java
    │   └── commands/
    │       └── CancelCommand.java
    ├── resources/
    │   ├── config.toml
    │   └── plugin.yml
    └── templates/net/demimine/dynamic/
        └── BuildConstants.java
```

**Plugin Features:**

1. **ServerPreConnectHandler**
   - Checks `demimine.authenticated` permission
   - Pings target server for quick availability check
   - If running: allows connection, reports join to API
   - If stopped: calls start API, queues player
   - **RAM Management** (NEW):
     - If start fails: gets list of auto-shutdown servers
     - Stops the server with shortest timer
     - Retries starting the target server
     - If still fails: denies connection
   - Dual queue mode:
     - Direct connections: send to queue server (nexus/limbo)
     - Hub connections: keep in hub with status messages
   - **Status Polling** (NEW):
     - Polls API status every `check_interval_seconds` (default 1s)
     - Uses `GET /api/servers/{name}/status` instead of just pinging
     - Waits up to `start_timeout_seconds` (default 120s)
   - Countdown teleport (5, 4, 3, 2, 1)

2. **DisconnectHandler**
   - Removes player from server player set
   - Reports leave to API
   - **Auto-Shutdown Check** (NEW):
     - Gets server status via API
     - Only starts auto-stop timer if `auto_shutdown_minutes > 0`
     - Only starts timer if server is empty

3. **ServerConnectedHandler**
   - Cancels pending auto-stop timer
   - Adds player to server player set

4. **QueueManager**
   - Tracks players waiting for servers
   - Manages countdown timers
   - Supports multiple queues per server
   - Methods: `addToQueue`, `removeFromQueue`, `startCountdown`, `cancelCountdown`, `getQueuedServer`

5. **AutoStopManager**
   - Tracks players per server
   - Manages auto-stop timers
   - Configurable timeout (default 15 minutes)
   - Methods: `addPlayerToServer`, `removePlayerFromServer`, `cancelStopTimer`, `scheduleStopTimer`
   - **NOTE**: `scheduleStopTimer` is now called externally (by DisconnectHandler) after checking auto-shutdown eligibility

6. **CancelCommand** (`/cancel`)
   - Removes player from queue
   - If queue becomes empty: calls stop API to cancel server start
   - In queue server: disconnects with "Player canceled the queue!"
   - In hub server: sends "Canceled queue for {server}!", keeps player in hub

7. **ApiClient**
   - `startServer(serverName)` - Calls `/api/servers/{name}/start`
   - `stopServer(serverId)` - Calls `/api/servers/{id}/stop`
   - `stopServerByName(serverName)` - Calls `/api/servers/{name}/stop-by-name`
   - `getAutoShutdownServers()` - Calls `/api/servers/auto-shutdown` (NEW)
   - `getServerStatus(serverName)` - Calls `/api/servers/{name}/status` (NEW)
   - `reportPlayerJoin(uuid, name, serverName)` - Calls `/api/players/join`
   - `reportPlayerLeave(uuid, serverName)` - Calls `/api/players/leave`
   - Includes inner classes: `AutoShutdownServer` and `ServerStatus` (NEW)

**Configuration (`config.toml`):**
```toml
manager_url = "http://host.docker.internal:8080"
api_key = ""  # Generate from Manager API keys page

auth_permission = "demimine.authenticated"

queue_server = "nexus"  # Queue server for direct connections (default: nexus)
hub_server = "nexus"    # Hub server name for in-hub queuing
use_hub_queue = true     # Enable in-hub queuing

check_interval_seconds = 1   # Poll interval for server status (default: 1s)
start_timeout_seconds = 120    # Timeout waiting for server to start (default: 120s)
auto_stop_timeout_minutes = 15  # Auto-stop idle servers after N minutes (default: 15min)
message_interval_seconds = 5     # Send loading message every N seconds

[messages]
starting = "<yellow>Server is starting..."
loading = "<yellow>Loading server..."
teleporting = "<green>Teleporting in 5 seconds!"
countdown = "<yellow>Teleporting in {seconds}..."
cancel_queue = "<red>Canceled queue for {server}!"
cancel_disconnect = "<red>Player canceled the queue!"
```

**Key Changes from Design Spec:**

1. ✅ **RAM Management on Start Failure**
   - When `POST /api/servers/{name}/start` fails (e.g., insufficient RAM)
   - Plugin calls `GET /api/servers/auto-shutdown` to get servers with `auto_shutdown_minutes > 0`
   - Stops the server with shortest timer via `POST /api/servers/{id}/stop`
   - Retries starting the target server
   - If still fails: denies connection

2. ✅ **Server Status Polling**
   - Uses `GET /api/servers/{name}/status` instead of just pinging
   - Polls every `check_interval_seconds` (default 1s, matches design)
   - Waits up to `start_timeout_seconds` (default 120s, matches design)
   - Returns when status is "running"

3. ✅ **Auto-Shutdown Eligibility Check**
   - `DisconnectHandler` now calls `GET /api/servers/{name}/status`
   - Only starts auto-stop timer if `auto_shutdown_minutes > 0`
   - Only starts timer if server is empty (no players)

4. ✅ **Config Names Match Design**
   - `check_interval_seconds` (was `poll_interval_seconds`)
   - `start_timeout_seconds` (was missing, new)
   - `queue_server` default changed to "nexus" (was "queue")
   - Kept `auto_stop_timeout_minutes`, `message_interval_seconds` as they were reasonable additions

## Build & Deploy

The backend has been rebuilt and deployed:
```bash
docker compose build --no-cache
docker compose up -d
```

**Note:** The DemiDynamic plugin needs to be built separately with Gradle (not available in current environment). Instructions are in `DemiDynamic/README.md`.

## Testing

Once built, the plugin can be tested by:

1. Creating an API key in the Manager web UI
2. Configuring the plugin with the API key
3. Trying to connect to a stopped server
4. Verifying auto-start and queue behavior
5. Testing `/cancel` command in both queue and hub servers
6. Verifying auto-stop after timeout when no players are on a server
7. **Testing RAM management:**
   - Fill RAM with multiple servers
   - Try to start another server
   - Verify it stops an auto-shutdown server and retries
8. **Testing status polling:**
   - Watch logs for API status calls
   - Verify it checks every `check_interval_seconds`

## Files Modified/Created

**Backend:**
- `manager/internal/api/handlers/players.go` (new, with additional endpoints)
- `manager/internal/api/router.go` (modified)

**Plugin:**
- All files in `DemiDynamic/` directory (new, with RAM management and status polling)

## Design Spec Compliance

✅ All requirements from `design.md` lines 1722-1763 are now implemented:

- ✅ Player attempts to connect to stopped server
- ✅ Plugin intercepts `ServerPreConnectEvent` (watching ALL connection attempts)
- ✅ Check if player has `demimine.authenticated` permission
- ✅ If not authenticated, deny with message
- ✅ Call Manager API: `POST /api/servers/:name/start`
- ✅ If error (insufficient RAM): check servers with `auto_shutdown` active, stop shortest-timer server, retry start, deny if still fails
- ✅ While starting: send player to queue server or show "Starting server..." message
- ✅ Poll server status every 1 second
- ✅ On `running`, allow connection
- ✅ Send player info to Manager API: `POST /api/players/join`
- ✅ Player leaves server → report leave to API
- ✅ Check if auto-shutdown enabled for that server
- ✅ Start 15-minute timer
- ✅ If player rejoins, cancel timer
- ✅ On timer expiry, call Manager API: `POST /api/servers/:id/stop`
- ✅ Config: `check_interval_seconds = 1`, `start_timeout_seconds = 120`, `queue_server = "nexus"`

## Next Steps

To use the DemiDynamic plugin:

1. Build the plugin JAR using Gradle:
   ```bash
   cd DemiDynamic
   ./gradlew shadowJar
   ```

2. Place `build/libs/DemiDynamic-1.0.0.jar` in the Velocity proxy's `plugins` folder

3. Create or configure a queue server (limbo/nexus) for direct connections

4. Configure the hub server name in the plugin config

5. Set up an API key in the Manager web UI and configure it in the plugin

6. Restart the proxy

7. Test auto-start/stop functionality
8. Test RAM management by starting multiple servers
9. Test `/cancel` command in both queue and hub servers
10. Verify auto-stop after timeout when no players are on a server
