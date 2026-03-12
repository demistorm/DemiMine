package handlers

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"github.com/demimine/manager/internal/mc"
)

func AssignServerToProxy(db *sql.DB, serverID int64, proxyID int64, serversDir string) (int, error) {
	var serverName, serverType, mcVersion string
	err := db.QueryRow("SELECT name, type, version FROM servers WHERE id = ?", serverID).Scan(&serverName, &serverType, &mcVersion)
	if err != nil {
		return 0, fmt.Errorf("server not found: %w", err)
	}

	var proxyName string
	var forwardingSecret string
	err = db.QueryRow("SELECT name, forwarding_secret FROM proxies WHERE id = ?", proxyID).Scan(&proxyName, &forwardingSecret)
	if err != nil {
		return 0, fmt.Errorf("proxy not found: %w", err)
	}

	var serverCount int
	err = db.QueryRow("SELECT COUNT(*) FROM servers WHERE proxy_id = ?", proxyID).Scan(&serverCount)
	if err != nil {
		serverCount = 0
	}

	port := 30000 + int(proxyID*100) + serverCount + 1

	_, err = db.Exec(`
		UPDATE servers SET proxy_id = ?, host_port = ?, domain = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, proxyID, port, nil, serverID)
	if err != nil {
		return 0, fmt.Errorf("failed to update server: %w", err)
	}

	serverPath := filepath.Join(serversDir, serverName)
	props, err := mc.ReadServerProperties(filepath.Join(serverPath, "server.properties"))
	if err != nil {
		props = mc.DefaultServerProperties()
	}

	props.Set("online-mode", "false")
	props.Set("server-port", fmt.Sprintf("%d", port))

	if err := props.WriteToFile(filepath.Join(serverPath, "server.properties")); err != nil {
		return port, fmt.Errorf("failed to update server.properties: %w", err)
	}

	if err := ConfigureServerProxy(serverPath, serverType, mcVersion, forwardingSecret); err != nil {
		return port, fmt.Errorf("failed to configure server proxy settings: %w", err)
	}

	if err := SyncProxyConfig(db, proxyID, serversDir); err != nil {
		return port, fmt.Errorf("failed to sync proxy config: %w", err)
	}

	return port, nil
}

func RemoveServerFromProxy(db *sql.DB, serverID int64, serversDir string) error {
	var serverName, serverType, mcVersion string
	var proxyID sql.NullInt64
	err := db.QueryRow("SELECT name, type, version, proxy_id FROM servers WHERE id = ?", serverID).Scan(&serverName, &serverType, &mcVersion, &proxyID)
	if err != nil {
		return fmt.Errorf("server not found: %w", err)
	}

	if !proxyID.Valid {
		return nil
	}

	oldProxyID := proxyID.Int64

	_, err = db.Exec(`
		UPDATE servers SET proxy_id = NULL, host_port = NULL, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, serverID)
	if err != nil {
		return fmt.Errorf("failed to update server: %w", err)
	}

	serverPath := filepath.Join(serversDir, serverName)
	props, err := mc.ReadServerProperties(filepath.Join(serverPath, "server.properties"))
	if err != nil {
		props = mc.DefaultServerProperties()
	}

	props.Set("online-mode", "true")
	props.WriteToFile(filepath.Join(serverPath, "server.properties"))

	if err := RevertServerProxyConfig(serverPath, serverType, mcVersion); err != nil {
		fmt.Printf("Warning: failed to revert server proxy config: %v\n", err)
	}

	if err := SyncProxyConfig(db, oldProxyID, serversDir); err != nil {
		return fmt.Errorf("failed to sync proxy config: %w", err)
	}

	return nil
}

func SyncProxyConfig(db *sql.DB, proxyID int64, serversDir string) error {
	var proxyName string
	var hostPort int
	var forwardingSecret string
	err := db.QueryRow("SELECT name, host_port, forwarding_secret FROM proxies WHERE id = ?", proxyID).Scan(&proxyName, &hostPort, &forwardingSecret)
	if err != nil {
		return fmt.Errorf("proxy not found: %w", err)
	}

	rows, err := db.Query(`
		SELECT name, domain FROM servers WHERE proxy_id = ? ORDER BY id
	`, proxyID)
	if err != nil {
		return fmt.Errorf("failed to query servers: %w", err)
	}
	defer rows.Close()

	servers := make(map[string]string)
	forcedHosts := make(map[string][]string)
	var tryServers []string

	for rows.Next() {
		var name string
		var domain sql.NullString
		if err := rows.Scan(&name, &domain); err != nil {
			continue
		}

		containerName := "demimine-" + sanitizeNameForProxy(name)
		servers[name] = containerName + ":25565"

		if domain.Valid && domain.String != "" {
			forcedHosts[domain.String] = []string{name}
		}

		if len(tryServers) == 0 {
			tryServers = append(tryServers, name)
		}
	}

	proxyPath := filepath.Join(serversDir, proxyName)
	config := mc.DefaultVelocityConfig(hostPort, forwardingSecret)
	config.Servers = servers
	config.Try = tryServers
	config.ForcedHosts = forcedHosts

	return config.WriteToFile(filepath.Join(proxyPath, "velocity.toml"))
}

func sanitizeNameForProxy(name string) string {
	result := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			if c >= 'A' && c <= 'Z' {
				c = c + 32
			}
			result = append(result, c)
		} else if c == ' ' {
			result = append(result, '-')
		}
	}
	return string(result)
}

func ConfigureServerProxy(serverPath, serverType, mcVersion, forwardingSecret string) error {
	switch serverType {
	case "paper", "purpur":
		if err := mc.ConfigurePaperProxy(serverPath, forwardingSecret); err != nil {
			return fmt.Errorf("failed to configure paper proxy: %w", err)
		}
	case "fabric":
		if err := mc.ConfigureFabricProxy(serverPath, mcVersion, forwardingSecret); err != nil {
			return fmt.Errorf("failed to configure fabric proxy: %w", err)
		}
	case "neoforge":
		if err := mc.ConfigureForgeProxy(serverPath, mcVersion, "neoforge", forwardingSecret); err != nil {
			return fmt.Errorf("failed to configure neoforge proxy: %w", err)
		}
	case "forge":
		if err := mc.ConfigureForgeProxy(serverPath, mcVersion, "forge", forwardingSecret); err != nil {
			return fmt.Errorf("failed to configure forge proxy: %w", err)
		}
	}
	return nil
}

func RevertServerProxyConfig(serverPath, serverType, mcVersion string) error {
	switch serverType {
	case "paper", "purpur":
		if err := mc.RevertPaperProxyConfig(serverPath); err != nil {
			return fmt.Errorf("failed to revert paper proxy config: %w", err)
		}
	case "fabric":
		if err := mc.RevertFabricProxyConfig(serverPath); err != nil {
			return fmt.Errorf("failed to revert fabric proxy config: %w", err)
		}
	case "neoforge":
		if err := mc.RevertForgeProxyConfig(serverPath, mcVersion, "neoforge"); err != nil {
			return fmt.Errorf("failed to revert neoforge proxy config: %w", err)
		}
	case "forge":
		if err := mc.RevertForgeProxyConfig(serverPath, mcVersion, "forge"); err != nil {
			return fmt.Errorf("failed to revert forge proxy config: %w", err)
		}
	}
	return nil
}
