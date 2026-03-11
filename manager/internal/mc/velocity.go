package mc

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type VelocityConfig struct {
	Bind             string
	MOTD             string
	ShowMaxPlayers   int
	OnlineMode       bool
	ForwardingMode   string
	ForwardingSecret string
	Servers          map[string]string
	Try              []string
	ForcedHosts      map[string][]string
}

func DownloadVelocityJar(destPath string) error {
	resp, err := httpClient.Get("https://api.papermc.io/v2/projects/velocity")
	if err != nil {
		return fmt.Errorf("failed to get velocity versions: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		VersionGroups []string `json:"version_groups"`
		Versions      []string `json:"versions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode velocity versions: %w", err)
	}

	if len(data.Versions) == 0 {
		return fmt.Errorf("no velocity versions available")
	}

	latestVersion := data.Versions[len(data.Versions)-1]

	buildResp, err := httpClient.Get(fmt.Sprintf("https://api.papermc.io/v2/projects/velocity/versions/%s", latestVersion))
	if err != nil {
		return fmt.Errorf("failed to get velocity builds: %w", err)
	}
	defer buildResp.Body.Close()

	var buildData struct {
		Builds []int `json:"builds"`
	}
	if err := json.NewDecoder(buildResp.Body).Decode(&buildData); err != nil {
		return fmt.Errorf("failed to decode velocity builds: %w", err)
	}

	if len(buildData.Builds) == 0 {
		return fmt.Errorf("no builds available for velocity %s", latestVersion)
	}

	latestBuild := buildData.Builds[len(buildData.Builds)-1]

	downloadURL := fmt.Sprintf(
		"https://api.papermc.io/v2/projects/velocity/versions/%s/builds/%d/downloads/velocity-%s-%d.jar",
		latestVersion, latestBuild, latestVersion, latestBuild,
	)

	return downloadFile(downloadURL, destPath)
}

func GenerateForwardingSecret() (string, error) {
	bytes := make([]byte, 6)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func WriteForwardingSecret(proxyPath, secret string) error {
	secretPath := filepath.Join(proxyPath, "forwarding.secret")
	return os.WriteFile(secretPath, []byte(secret+"\n"), 0600)
}

func DefaultVelocityConfig(hostPort int, forwardingSecret string) *VelocityConfig {
	return &VelocityConfig{
		Bind:             fmt.Sprintf("0.0.0.0:%d", hostPort),
		MOTD:             "DemiMine Proxy",
		ShowMaxPlayers:   10,
		OnlineMode:       true,
		ForwardingMode:   "modern",
		ForwardingSecret: forwardingSecret,
		Servers:          make(map[string]string),
		Try:              []string{},
		ForcedHosts:      make(map[string][]string),
	}
}

func (c *VelocityConfig) WriteToFile(path string) error {
	var sb strings.Builder

	sb.WriteString(`# Config version. Do not change this
config-version = "2.7"

# What port should the proxy be bound to? By default, we'll bind to all addresses on port 25565.
bind = "` + c.Bind + `"

# What should be the MOTD? This gets displayed when the player adds your server to
# their server list. Only MiniMessage format is accepted.
motd = "` + c.MOTD + `"

# What should we display for the maximum number of players? (Velocity does not support a cap
# on the number of players online.)
show-max-players = ` + fmt.Sprintf("%d", c.ShowMaxPlayers) + `

# Should we authenticate players with Mojang? By default, this is on.
online-mode = ` + fmt.Sprintf("%v", c.OnlineMode) + `

# Should the proxy enforce the new public key security standard? By default, this is on.
force-key-authentication = true

# If client's ISP/AS sent from this proxy is different from the one from Mojang's
# authentication server, the player is kicked. This disallows some VPN and proxy
# connections but is a weak form of protection.
prevent-client-proxy-connections = false

# Should we forward IP addresses and other data to backend servers?
# Available options:
# - "none":        No forwarding will be done. All players will appear to be connecting
#                  from the proxy and will have offline-mode UUIDs.
# - "legacy":      Forward player IPs and UUIDs in a BungeeCord-compatible format. Use this
#                  if you run servers using Minecraft 1.12 or lower.
# - "bungeeguard": Forward player IPs and UUIDs in a format supported by the BungeeGuard
#                  plugin. Use this if you run servers using Minecraft 1.12 or lower, and are
#                  unable to implement network level firewalling (on a shared host).
# - "modern":      Forward player IPs and UUIDs as part of the login process using
#                  Velocity's native forwarding. Only applicable for Minecraft 1.13 or higher.
player-info-forwarding-mode = "` + c.ForwardingMode + `"

# If you are using modern or BungeeGuard IP forwarding, configure a file that contains a unique secret here.
# The file is expected to be UTF-8 encoded and not empty.
forwarding-secret-file = "forwarding.secret"

# Announce whether or not your server supports Forge. If you run a modded server, we
# suggest turning this on.
# 
# If your network runs one modpack consistently, consider using ping-passthrough = "mods"
# instead for a nicer display in the server list.
announce-forge = false

# If enabled (default is false) and the proxy is in online mode, Velocity will kick
# any existing player who is online if a duplicate connection attempt is made.
kick-existing-players = false

# Should Velocity pass server list ping requests to a backend server?
# Available options:
# - "disabled":    No pass-through will be done. The velocity.toml and server-icon.png
#                  will determine the initial server list ping response.
# - "mods":        Passes only the mod list from your backend server into the response.
#                  The first server in your try list (or forced host) with a mod list will be
#                  used. If no backend servers can be contacted, Velocity won't display any
#                  mod information.
# - "description": Uses the description and mod list from the backend server. The first
#                  server in your try (or forced host) list that responds is used for the
#                  description and mod list.
# - "all":         Uses the backend server's response as the proxy response. The Velocity
#                  configuration is used if no servers could be contacted.
ping-passthrough = "all"

# If not enabled (default is true) player IP addresses will be replaced by <ip address withheld> in logs
enable-player-address-logging = true

[servers]
# Configure your servers here. Each key represents the server's name, and the value
# represents the IP address of the server to connect to.
`)

	names := make([]string, 0, len(c.Servers))
	for name := range c.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		sb.WriteString(fmt.Sprintf("%s = \"%s\"\n", name, c.Servers[name]))
	}

	sb.WriteString(`
# In what order we should try servers when a player logs in or is kicked from a server.
try = [
`)
	for i, server := range c.Try {
		sb.WriteString(fmt.Sprintf("    \"%s\"", server))
		if i < len(c.Try)-1 {
			sb.WriteString(",")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("]\n")

	sb.WriteString(`
[forced-hosts]
# Configure your forced hosts here.
`)

	for host, servers := range c.ForcedHosts {
		sb.WriteString(fmt.Sprintf("\"%s\" = [\n", host))
		for i, server := range servers {
			sb.WriteString(fmt.Sprintf("    \"%s\"", server))
			if i < len(servers)-1 {
				sb.WriteString(",")
			}
			sb.WriteString("\n")
		}
		sb.WriteString("]\n")
	}

	sb.WriteString(`
[advanced]
# How large a Minecraft packet has to be before we compress it. Setting this to zero will
# compress all packets, and setting it to -1 will disable compression entirely.
compression-threshold = 256

# How much compression should be done (from 0-9). The default is -1, which uses the
# default level of 6.
compression-level = -1

# How fast (in milliseconds) are clients allowed to connect after the last connection? By
# default, this is three seconds. Disable this by setting this to 0.
login-ratelimit = 3000

# Specify a custom timeout for connection timeouts here. The default is five seconds.
connection-timeout = 20000

# Specify a read timeout for connections here. The default is 30 seconds.
read-timeout = 90000

# Enables compatibility with HAProxy's PROXY protocol. If you don't know what this is for, then
# don't enable it.
haproxy-protocol = false

# Enables TCP fast open support on the proxy. Requires the proxy to run on Linux.
tcp-fast-open = false

# Enables BungeeCord plugin messaging channel support on Velocity.
bungee-plugin-message-channel = true

# Shows ping requests to the proxy from clients.
show-ping-requests = false

# By default, Velocity will attempt to gracefully handle situations where the user unexpectedly
# loses connection to the server without an explicit disconnect message by attempting to fall the
# user back, except in the case of read timeouts. BungeeCord will disconnect the user instead. You
# can disable this setting to use the BungeeCord behavior.
failover-on-unexpected-server-disconnect = true

# Declares the proxy commands to 1.13+ clients.
announce-proxy-commands = true

# Enables the logging of commands
log-command-executions = false

# Enables logging of player connections when connecting to the proxy, switching servers
# and disconnecting from the proxy.
log-player-connections = true

# Allows players transferred from other hosts via the
# Transfer packet (Minecraft 1.20.5) to be received.
accepts-transfers = false

[query]
# Whether to enable responding to GameSpy 4 query responses or not.
enabled = false

# If query is enabled, on what port should the query protocol listen on?
port = 25565

# This is the map name that is reported to the query services.
map = "Velocity"

# Whether plugins should be shown in query response by default or not
show-plugins = false
`)

	return os.WriteFile(path, []byte(sb.String()), 0644)
}

func ReadVelocityConfig(path string) (*VelocityConfig, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read velocity.toml: %w", err)
	}

	config := &VelocityConfig{
		Servers:     make(map[string]string),
		Try:         []string{},
		ForcedHosts: make(map[string][]string),
	}

	lines := strings.Split(string(content), "\n")
	section := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.Trim(line, "[]")
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, "\"")

		switch section {
		case "":
			switch key {
			case "bind":
				config.Bind = value
			case "motd":
				config.MOTD = value
			case "show-max-players":
				fmt.Sscanf(value, "%d", &config.ShowMaxPlayers)
			case "online-mode":
				config.OnlineMode = value == "true"
			case "player-info-forwarding-mode":
				config.ForwardingMode = value
			}
		case "servers":
			config.Servers[key] = value
		case "forced-hosts":
			if strings.HasPrefix(value, "[") {
				serverList := strings.Trim(value, "[]")
				servers := strings.Split(serverList, ",")
				for i, s := range servers {
					servers[i] = strings.Trim(strings.TrimSpace(s), "\"")
				}
				if len(servers) > 0 && servers[0] != "" {
					config.ForcedHosts[key] = servers
				}
			}
		}
	}

	return config, nil
}
