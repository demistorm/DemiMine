package handlers

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/demimine/manager/internal/mc"
)

func AssignServerToProxy(db *sql.DB, serverID int64, proxyID int64, serversDir string) (int, error) {
	var serverName, serverSanitizedName, serverType, mcVersion string
	err := db.QueryRow("SELECT name, sanitized_name, type, version FROM servers WHERE id = ?", serverID).Scan(&serverName, &serverSanitizedName, &serverType, &mcVersion)
	if err != nil {
		return 0, fmt.Errorf("server not found: %w", err)
	}

	var proxyName, proxySanitizedName string
	var forwardingSecret string
	err = db.QueryRow("SELECT name, sanitized_name, forwarding_secret FROM proxies WHERE id = ?", proxyID).Scan(&proxyName, &proxySanitizedName, &forwardingSecret)
	if err != nil {
		return 0, fmt.Errorf("proxy not found: %w", err)
	}

	basePort := 30000 + int(proxyID*100)
	var maxPort sql.NullInt64
	err = db.QueryRow("SELECT COALESCE(MAX(host_port), ?) FROM servers WHERE proxy_id = ?", basePort, proxyID).Scan(&maxPort)
	if err != nil {
		maxPort.Int64 = int64(basePort)
	}

	port := int(maxPort.Int64) + 1

	_, err = db.Exec(`
		UPDATE servers SET proxy_id = ?, host_port = ?, domain = ?, updated_at = CURRENT_TIMESTAMP 
		WHERE id = ?
	`, proxyID, port, nil, serverID)
	if err != nil {
		return 0, fmt.Errorf("failed to update server: %w", err)
	}

	serverPath := filepath.Join(serversDir, serverSanitizedName)
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

	if isAuthServer(serverName, serverType) {
		proxyPath := filepath.Join(serversDir, proxySanitizedName)
		if err := updateDemiAuthLoginServer(proxyPath, serverSanitizedName); err != nil {
			fmt.Printf("Warning: failed to update DemiAuth config: %v\n", err)
		}
	}

	return port, nil
}

func RemoveServerFromProxy(db *sql.DB, serverID int64, serversDir string) error {
	var serverName, serverSanitizedName, serverType, mcVersion string
	var proxyID sql.NullInt64
	err := db.QueryRow("SELECT name, sanitized_name, type, version, proxy_id FROM servers WHERE id = ?", serverID).Scan(&serverName, &serverSanitizedName, &serverType, &mcVersion, &proxyID)
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

	serverPath := filepath.Join(serversDir, serverSanitizedName)
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
	var proxySanitizedName string
	var hostPort int
	var forwardingSecret string
	err := db.QueryRow("SELECT sanitized_name, host_port, forwarding_secret FROM proxies WHERE id = ?", proxyID).Scan(&proxySanitizedName, &hostPort, &forwardingSecret)
	if err != nil {
		return fmt.Errorf("proxy not found: %w", err)
	}

	rows, err := db.Query(`
		SELECT sanitized_name, domain, host_port FROM servers WHERE proxy_id = ? ORDER BY id
	`, proxyID)
	if err != nil {
		return fmt.Errorf("failed to query servers: %w", err)
	}
	defer rows.Close()

	servers := make(map[string]string)
	forcedHosts := make(map[string][]string)
	var tryServers []string

	for rows.Next() {
		var sanitizedName string
		var domain sql.NullString
		var hostPort int
		if err := rows.Scan(&sanitizedName, &domain, &hostPort); err != nil {
			continue
		}

		containerName := "demimine-" + sanitizedName
		servers[sanitizedName] = fmt.Sprintf("%s:%d", containerName, hostPort)

		if domain.Valid && domain.String != "" {
			forcedHosts[domain.String] = []string{sanitizedName}
		}

		if len(tryServers) == 0 {
			tryServers = append(tryServers, sanitizedName)
		}
	}

	proxyPath := filepath.Join(serversDir, proxySanitizedName)
	config := mc.DefaultVelocityConfig(hostPort, forwardingSecret)
	config.Servers = servers
	config.Try = tryServers
	config.ForcedHosts = forcedHosts

	return config.WriteToFile(filepath.Join(proxyPath, "velocity.toml"))
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

func isAuthServer(serverName, serverType string) bool {
	if strings.ToLower(serverType) != "nanolimbo" {
		return false
	}
	lowerName := strings.ToLower(serverName)
	return strings.Contains(lowerName, "auth") || strings.Contains(lowerName, "login")
}

func hasDemiAuthLoginServerConfigured(proxyPath string) bool {
	configPath := filepath.Join(proxyPath, "plugins", "demiauth", "config.toml")

	content, err := os.ReadFile(configPath)
	if err != nil {
		return false
	}

	configStr := string(content)
	return !strings.Contains(configStr, `loginServer = "login"`) && !strings.Contains(configStr, `loginServer="login"`)
}
