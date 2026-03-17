# DemiDynamic

A Velocity plugin that automatically starts and stops Minecraft servers based on player activity.

## Features

- **Auto-start**: Starts servers when players try to connect (if not already running)
- **Auto-stop**: Stops idle servers after a configurable timeout
- **RAM management**: Automatically stops a server with auto-shutdown enabled if start fails due to insufficient RAM, then retries
- **Player tracking**: Reports player joins/leaves to Manager API for accurate counts
- **Dual queue system**:
  - Direct connections → players sent to queue server (limbo)
  - In-hub connections → players stay in hub with status messages and countdown
- **Queue cancel**: Players can cancel their queue with `/cancel` command
- **Status polling**: Polls Manager API for server status with configurable intervals
- **Auto-shutdown check**: Only starts auto-stop timers for servers with `auto_shutdown_minutes > 0`

## Requirements

- Velocity 3.4+ proxy
- DemiAuth plugin (for `demimine.authenticated` permission)
- Manager API running with API key configured

## Building

```bash
# Using Gradle wrapper
./gradlew shadowJar

# Or using system gradle
gradle shadowJar
```

The compiled JAR will be in `build/libs/DemiDynamic-1.0.0.jar`.

## Installation

1. Place compiled JAR in your Velocity proxy's `plugins` folder
2. Restart the proxy
3. Configure `plugins/DemiDynamic/config.toml`

## Configuration

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

## Flow

### Server Start Flow

1. Player attempts to connect to a server
2. Plugin checks `demimine.authenticated` permission
3. If not authenticated: deny connection
4. If server is running: allow connection, report join to API
5. If server is stopped:
   - Call `POST /api/servers/{name}/start`
   - If start fails (insufficient RAM):
     - Get list of running servers with `auto_shutdown_minutes > 0` from `GET /api/servers/auto-shutdown`
     - Stop the one with shortest timer
     - Retry starting the server
     - If still fails: deny connection
   - Queue player (send to queue server or keep in hub)
   - Poll server status every `check_interval_seconds` via `GET /api/servers/{name}/status`
   - When status is "running": allow connection, report join to API

### Server Stop Flow

1. Player disconnects from server
2. Remove player from server's player set
3. Report leave to API via `POST /api/players/leave`
4. Check if server has `auto_shutdown_minutes > 0` via API status
5. If yes and server is empty: start auto-stop timer

## Commands

- `/cancel` - Cancel your current queue
  - In queue server: Disconnects you with "Player canceled the queue!"
  - In hub server: Cancels the queue, keeps you in hub with "Canceled queue for {server}!"
  - If queue becomes empty: cancels server start by stopping the server

## API Endpoints

The plugin uses the following Manager API endpoints:

- `POST /api/servers/{name}/start` - Start a server
- `POST /api/servers/{name}/stop-by-name` - Stop a server by name
- `GET /api/servers/{name}/status` - Get server status (including auto_shutdown_minutes, player_count)
- `GET /api/servers/auto-shutdown` - Get list of running servers with auto_shutdown_minutes > 0
- `POST /api/players/join` - Report player joined server
- `POST /api/players/leave` - Report player left server
- `GET /api/servers/{id}/players` - Get players on a server (for web UI)

## Permissions

- `demimine.authenticated` - Required to trigger auto-start (granted by DemiAuth)

## License

MIT License
