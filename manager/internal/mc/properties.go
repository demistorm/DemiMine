package mc

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"
)

type ServerProperties map[string]string

func DefaultServerProperties() ServerProperties {
	return ServerProperties{
		"server-port":        "25565",
		"online-mode":        "false",
		"enable-rcon":        "false",
		"enable-query":       "false",
		"snooper-enabled":    "false",
		"hardcore":           "false",
		"allow-flight":       "true",
		"spawn-protection":   "0",
		"max-tick-time":      "60000",
		"motd":               "A DemiMine Server",
		"max-players":        "20",
		"difficulty":         "normal",
		"gamemode":           "survival",
		"pvp":                "true",
		"level-name":         "world",
		"view-distance":      "10",
		"simulation-distance": "10",
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
