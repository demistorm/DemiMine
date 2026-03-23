package mc

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type ServerProperties map[string]string

func DefaultServerProperties() ServerProperties {
	return ServerProperties{
		"server-port":           "25565",
		"online-mode":           "true",
		"enable-rcon":           "true",
		"rcon.port":             "39521",
		"rcon.password":         os.Getenv("RCON_PASSWORD"),
		"broadcast-rcon-to-ops": "false",
		"enable-query":          "false",
		"hardcore":              "false",
		"allow-flight":          "true",
		"spawn-protection":      "0",
		"max-tick-time":         "60000",
		"motd":                  "A DemiMine Server",
		"max-players":           "20",
		"difficulty":            "normal",
		"gamemode":              "survival",
		"pvp":                   "true",
		"level-name":            "world",
		"view-distance":         "12",
		"simulation-distance":   "7",
	}
}

func (sp ServerProperties) Set(key, value string) {
	sp[key] = value
}

func (sp ServerProperties) Get(key string) string {
	return sp[key]
}

func (sp ServerProperties) WriteToFile(path string) error {
	var buf bytes.Buffer

	keys := make([]string, 0, len(sp))
	for k := range sp {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		buf.WriteString(fmt.Sprintf("%s=%s\n", k, sp[k]))
	}

	return os.WriteFile(path, buf.Bytes(), 0644)
}

func ReadServerProperties(path string) (ServerProperties, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	props := make(ServerProperties)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			props[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}

	return props, scanner.Err()
}

func MergeServerProperties(base, override ServerProperties) ServerProperties {
	result := make(ServerProperties)
	for k, v := range base {
		result[k] = v
	}
	for k, v := range override {
		result[k] = v
	}
	return result
}

func DefaultNanoLimboConfig(velocitySecret string, port int) string {
	config := fmt.Sprintf(`#
# NanoLimbo configuration
#

# Server's host address and port. Set ip empty to use public address
bind:
  ip: '0.0.0.0'
  port: {{AUTO_ASSIGNED_PORT}}

# Max number of players can join to server
# Set -1 to make it infinite
maxPlayers: -1

# Server's data in servers list
ping:
  description: '{"text": "&9DemiMine Login"}'
  version: 'Login'
  protocol: -1

# Available dimensions: OVERWORLD, NETHER, THE_END
dimension: THE_END

# Whether to display the player in the player list
playerList:
  enable: false
  username: 'Limboland'

# Whether to display header and footer in the player list
headerAndFooter:
  enable: false
  header: '{"text": "&eWelcome!"}'
  footer: '{"text": "&9DemiMine"}'

# Setup player's game mode
# 0 - Survival
# 1 - Creative (hide HP and food bar)
# 2 - Adventure
# 3 - Spectator (hide all UI bars)
gameMode: 3

# Server name which is shown under F3
brandName:
  enable: true
  content: 'DemiAuth'

# Message sends when player joins to the server
joinMessage:
  enable: true
  text: '{"text": "&eType password in chat to continue"}'

# BossBar displays when player joins to the server
bossBar:
  enable: true
  text: '{"text": "&6Authentication Required"}'
  health: 1.0
  color: YELLOW
  division: SOLID

# Display title and subtitle
title:
  enable: true
  title: '{"text": "&9&lWelcome!"}'
  subtitle: '{"text": "&6Please enter password"}'
  fadeIn: 10
  stay: 100
  fadeOut: 10

# Player info forwarding support
infoForwarding:
  type: MODERN
  secret: '%s'
 `, velocitySecret)
	return strings.Replace(config, "{{AUTO_ASSIGNED_PORT}}", strconv.Itoa(port), 1)
}

func WriteNanoLimboConfig(serverPath, velocitySecret string, port int) error {
	configPath := filepath.Join(serverPath, "settings.yml")
	configContent := DefaultNanoLimboConfig(velocitySecret, port)
	return os.WriteFile(configPath, []byte(configContent), 0644)
}
