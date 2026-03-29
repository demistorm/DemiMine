# DemiMine
NOTE: This is not some fancy production what have you, this was "vibecoded" by me for my personal use being specifically suited for Minecraft servers and networks running on home computers. It may fit a large variety of usecases and I hope it can be helpful for somebody :) For all intents and purposes, this is just uploaded here for my convienence and if it is helpful for somebody, awesome! Much of the README was also written by AI but at a glance it looks accurate enough. Anyways, there's my "warning" haha. 

A fully-isolated Minecraft proxy and server manager with a web UI, deployed via Docker Compose.

DemiMine manages multiple Minecraft backend servers (Paper, Purpur, Fabric, NeoForge, Forge) behind one or more Velocity proxies. Servers are dynamically started/stopped based on player activity, with lazy-loaded Java runtimes and plugin management via Modrinth.

## Quick Start

### 1. Create a docker-compose.yml

```yaml
name: demimine

networks:
  demimine_internal:
    driver: bridge

services:
  manager:
    image: ghcr.io/demistorm/demimine:main
    container_name: demimine-manager
    restart: unless-stopped
    stop_grace_period: 60s
    ports:
      - "8025:8080"
    volumes:
      - ./data:/data
      - ./servers:/servers
      - ./backups:/backups
      - ./java:/java
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - TZ=America/Chicago
      - DEMIMINE_HOST_SERVERS_DIR=$PWD/servers
      - DEMIMINE_NETWORK=demimine_demimine_internal
      - DEMIMINE_MAX_RAM_MB=8192
      - RCON_PASSWORD=change_me_to_something_secure
    logging:
      driver: json-file
      options:
        max-size: "10m"
        max-file: "3"
    labels:
      - "demimine.managed=true"
      - "demimine.type=manager"
    networks:
      - demimine_internal
```

### 2. Start it up

```bash
docker compose up -d
```

### 3. Open the web UI

Go to `http://localhost:8025` and set up your admin username and password.

### 4. Create a proxy, then servers

1. Create a Velocity proxy (this is your player-facing server)
2. Create backend servers and assign them to the proxy
3. Players connect to the proxy port (default 25565)

## Configuration

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `RCON_PASSWORD` | *(required)* | Password for RCON access to all servers. Must be set. |
| `DEMIMINE_HOST_SERVERS_DIR` | `/servers` | Absolute path on host where server data is stored |
| `DEMIMINE_NETWORK` | `demimine_internal` | Docker network name for internal communication |
| `DEMIMINE_MAX_RAM_MB` | `16384` | Maximum total RAM for all Minecraft servers combined (this only applies if DemiDynamic is used) |
| `DEMIMINE_PORT` | `8080` | Port the web UI listens on inside the container |
| `TZ` | `America/Chicago` | Timezone for server containers |

### Port Mapping

The `8025:8080` mapping means the web UI is accessible at port `8025` on your host. Change `8025` to whatever port you prefer.

The Velocity proxy port (default `25565`) is set when creating a proxy in the web UI.

### Volume Paths

| Container Path | Default Host Path | Description |
|---|---|---|
| `/data` | `./data` | SQLite database and settings |
| `/servers` | `./servers` | Server files, configs, worlds |
| `/backups` | `./backups` | Server backup archives |
| `/java` | `./java` | Java runtimes |

## Building from Source

If you want to build the image yourself instead of pulling from ghcr.io:

1. Clone the repo
2. Build the DemiAuth and DemiDynamic plugins:
   ```bash
   cd DemiAuth && ./gradlew build && cd ..
   cd DemiDynamic && ./gradlew build && cd ..
   ```
3. Replace `image: ghcr.io/...` with `build: .` in your docker-compose.yml
4. Run `docker compose up -d --build`

## How It Works

```
Players connect to Velocity proxy (port 25565)
    |
    v
Velocity routes to backend servers on internal Docker network
    |
    v
DemiMine manager controls everything via Docker API
    |
    v
Web UI at port 8080 (mapped to 8025 on host)
```

- Backend servers run in their own Docker containers on an internal network (not exposed to the host)
- Only the Velocity proxy is exposed to players
- Servers start/stop automatically based on player activity (with DemiDynamic plugin)
- Java runtimes are downloaded on-demand based on server version
- Plugins can be installed/updated from Modrinth directly in the web UI

## Supported Server Types

- Paper
- Purpur
- Fabric
- NeoForge
- Forge

## Features

- Web UI with server/proxy canvas view
- Console log streaming with ANSI colors
- File browser with drag-and-drop upload
- Code editor with syntax highlighting
- Plugin management via Modrinth
- Backup system (scheduled and manual)
- Auto-shutdown idle servers
- Server duplication for safe updates
- Spark profiler integration
- MiniMOTD for server list customization
