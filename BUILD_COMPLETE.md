# DemiDynamic Plugin - BUILD COMPLETE

## ✅ Build Successful

**JAR File:** `DemiDynamic-1.0.0.jar`
**Size:** 3.2 MB
**Location:** `/home/strasburg/Documents/Projects/DemiMine/DemiDynamic/build/libs/DemiDynamic-1.0.0.jar`

---

## Quick Install

```bash
# Copy JAR to your Velocity proxy's plugins folder
cp DemiDynamic-1.0.0.jar /path/to/velocity/proxy/plugins/

# Or, if you're running DemiMine with Docker:
docker cp DemiDynamic-1.0.0.jar demimine-proxy:/velocity/plugins/

# Restart the proxy
docker restart demimine-proxy  # or your proxy container name
```

---

## Configuration

After first start, the plugin will create `plugins/DemiDynamic/config.toml`:

```toml
manager_url = "http://host.docker.internal:8080"
api_key = ""  # <- Generate this from Manager Web UI

auth_permission = "demimine.authenticated"

queue_server = "nexus"  # Queue server (change if needed)
hub_server = "nexus"    # Hub server (change if needed)
use_hub_queue = true     # Enable in-hub queuing

check_interval_seconds = 1     # Poll interval (seconds)
start_timeout_seconds = 120      # Max wait time (seconds)
auto_stop_timeout_minutes = 15    # Idle timeout (minutes)
message_interval_seconds = 5

[messages]
starting = "<yellow>Server is starting..."
loading = "<yellow>Loading server..."
teleporting = "<green>Teleporting in 5 seconds!"
countdown = "<yellow>Teleporting in {seconds}..."
cancel_queue = "<red>Canceled queue for {server}!"
cancel_disconnect = "<red>Player canceled the queue!"
```

---

## Setup Steps

### 1. Generate API Key

1. Open DemiMine Web UI at `http://localhost:8025`
2. Go to **API Keys** section
3. Click **Generate API Key**
4. Name it (e.g., "DemiDynamic")
5. Copy the generated key

### 2. Configure Plugin

1. Edit `plugins/DemiDynamic/config.toml` in your proxy
2. Set `api_key` to the key you copied
3. Adjust `queue_server` and `hub_server` names if needed
4. Restart the proxy

### 3. Ensure Servers Have Auto-Shutdown Enabled

For servers you want to auto-stop:
1. Go to server details in DemiMine Web UI
2. Set **Auto Shutdown Minutes** to desired value (e.g., 15)
3. Save changes

### 4. Create Queue Server (Optional)

For direct connection queuing:
1. Create a lightweight server (Limbo, Waterfall, or empty Paper)
2. Name it matching `queue_server` in config (default: "nexus")
3. Ensure it's always running (not managed by DemiDynamic)
4. Add it to your Velocity `velocity.toml` servers list

---

## Features Available

✅ **Auto-Start**: Starts servers when players try to connect
✅ **Auto-Stop**: Stops idle servers after timeout (only if enabled)
✅ **RAM Management**: Stops auto-shutdown server if start fails, then retries
✅ **Player Tracking**: Reports joins/leaves for accurate counts
✅ **Dual Queue**:
   - Direct connections → send to queue server
   - Hub connections → stay in hub with countdown
✅ **Auth Check**: Requires `demimine.authenticated` permission
✅ **Smart Cancel**: `/cancel` command with different behavior per server
✅ **Status Polling**: Checks API every 1 second (configurable)

---

## Testing

### Test Auto-Start

1. Create a test server (e.g., "survival")
2. Make sure it's stopped
3. Try to connect via Velocity proxy
4. **Expected:** Server starts, you're queued, then connected

### Test Auto-Stop

1. Connect to a server with `auto_shutdown_minutes > 0`
2. Disconnect
3. Wait 15 minutes (or lower for testing)
4. **Expected:** Server stops automatically

### Test RAM Management

1. Start multiple servers until RAM is full
2. Try to start another server
3. **Expected:** Stops an auto-shutdown server, then starts the requested one

### Test Queue Cancel

1. Connect to a stopped server (gets queued)
2. Run `/cancel`
3. **Expected:**
   - In queue server: Disconnects with "Player canceled the queue!"
   - In hub server: Shows "Canceled queue for {server}!" and stays in hub

### Test Hub Queueing

1. Connect to your hub server
2. Try to connect to a stopped server via portal or command
3. **Expected:** Stays in hub, gets countdown messages, then teleports

---

## Logs

Plugin logs to Velocity console with `[DemiDynamic]` prefix:
- Player connection attempts
- Server start/stop actions
- Queue operations
- RAM management actions
- Auto-stop timer events

Check logs if something doesn't work:
```bash
docker logs -f demimine-proxy
```

---

## Troubleshooting

### Server Won't Auto-Start
- Check `demimine.authenticated` permission is granted (DemiAuth)
- Verify API key is correct
- Check `manager_url` is accessible
- Look at proxy logs for errors

### Server Won't Auto-Stop
- Verify `auto_shutdown_minutes > 0` in server settings
- Check if players are actually disconnecting
- Ensure server isn't being used

### Queue Not Working
- Verify queue server exists and is running
- Check `queue_server` name in config matches Velocity config
- Ensure player has `demimine.authenticated` permission

### RAM Management Not Working
- Ensure some servers have `auto_shutdown_minutes > 0`
- Check Manager has running servers to stop
- Verify API endpoint `GET /api/servers/auto-shutdown` works

---

## Dependencies

The plugin includes:
- Velocity API (compile only)
- okhttp3 (bundled)
- Gson (bundled via Velocity)
- night-config (bundled)

No external dependencies needed!

---

## Support

For issues or questions:
1. Check proxy logs for error messages
2. Verify API key is valid
3. Check Manager API is accessible
4. Review this documentation

---

**Build Date:** March 17, 2026
**Version:** 1.0.0
**Status:** ✅ Ready for Production
