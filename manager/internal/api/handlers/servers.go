package handlers

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/demimine/manager/internal/mc"
	"github.com/demimine/manager/internal/minimotd"
	"github.com/demimine/manager/internal/models"
	"github.com/go-chi/chi/v5"
)

type ServerHandler struct {
	db             *sql.DB
	docker         *docker.Client
	consoleManager *docker.ConsoleManager
	cfg            *config.Config
	minimotdMgr    *minimotd.Manager
}

func NewServerHandler(db *sql.DB, dockerClient *docker.Client, consoleManager *docker.ConsoleManager, cfg *config.Config, minimotdMgr *minimotd.Manager) *ServerHandler {
	if dockerClient == nil {
		panic("dockerClient is nil in NewServerHandler")
	}
	return &ServerHandler{
		db:             db,
		docker:         dockerClient,
		consoleManager: consoleManager,
		cfg:            cfg,
		minimotdMgr:    minimotdMgr,
	}
}

func nullStringToPtr(ns sql.NullString) *string {
	if !ns.Valid {
		return nil
	}
	return &ns.String
}

func (h *ServerHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT s.id, s.name, s.type, s.version, s.proxy_id, p.name, s.ram_mb, s.domain,
		       s.backup_interval_days, s.auto_shutdown_minutes, s.scheduled_start, s.scheduled_stop,
		       s.host_port, s.status, s.canvas_x, s.canvas_y, s.jar_build, s.created_at,
		       COALESCE(pc.cnt, 0) as player_count, s.minimotd_line1, s.minimotd_line2
		FROM servers s
		LEFT JOIN proxies p ON s.proxy_id = p.id
		LEFT JOIN (SELECT server_id, COUNT(*) as cnt FROM players GROUP BY server_id) pc ON pc.server_id = s.id
		ORDER BY s.created_at DESC
	`)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to list servers"})
		return
	}
	defer rows.Close()

	servers := []models.ServerResponse{}
	for rows.Next() {
		var s models.ServerResponse
		var proxyID sql.NullInt64
		var proxyName sql.NullString
		var domain sql.NullString
		var scheduledStart sql.NullString
		var scheduledStop sql.NullString
		var hostPort sql.NullInt64
		var minimotdLine1 sql.NullString
		var minimotdLine2 sql.NullString

		err := rows.Scan(
			&s.ID, &s.Name, &s.Type, &s.Version, &proxyID, &proxyName, &s.RAMMB, &domain,
			&s.BackupIntervalDays, &s.AutoShutdownMinutes, &scheduledStart, &scheduledStop,
			&hostPort, &s.Status, &s.CanvasX, &s.CanvasY, &s.JarBuild, &s.CreatedAt, &s.PlayerCount,
			&minimotdLine1, &minimotdLine2,
		)
		if err != nil {
			continue
		}

		if proxyID.Valid {
			pid := int64(proxyID.Int64)
			s.ProxyID = &pid
		}
		s.ProxyName = nullStringToPtr(proxyName)
		s.Domain = nullStringToPtr(domain)
		s.ScheduledStart = nullStringToPtr(scheduledStart)
		s.ScheduledStop = nullStringToPtr(scheduledStop)
		if hostPort.Valid {
			hp := int(hostPort.Int64)
			s.HostPort = &hp
		}
		s.IconPath = h.getIconPath(s.ID, s.Name)
		s.MinimotdLine1 = nullStringToPtr(minimotdLine1)
		s.MinimotdLine2 = nullStringToPtr(minimotdLine2)

		servers = append(servers, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

type CreateServerRequest struct {
	Name                string  `json:"name"`
	Type                string  `json:"type"`
	Version             string  `json:"version"`
	ProxyID             *int64  `json:"proxy_id"`
	HostPort            *int    `json:"host_port"`
	RAMMB               int     `json:"ram_mb"`
	Domain              *string `json:"domain"`
	BackupIntervalDays  int     `json:"backup_interval_days"`
	AutoShutdownMinutes int     `json:"auto_shutdown_minutes"`
	ScheduledStart      *string `json:"scheduled_start"`
	ScheduledStop       *string `json:"scheduled_stop"`
	MinimotdLine1       *string `json:"minimotd_line1"`
	MinimotdLine2       *string `json:"minimotd_line2"`
}

func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.Type == "" || req.Version == "" || req.RAMMB == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name, type, version, and ram_mb are required"})
		return
	}

	if req.ProxyID == nil && req.HostPort == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "either proxy_id or host_port must be specified"})
		return
	}

	force := r.URL.Query().Get("force") == "true"

	if req.HostPort != nil && !force {
		var existingName string
		err := h.db.QueryRow("SELECT name FROM servers WHERE host_port = ?", *req.HostPort).Scan(&existingName)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "port_in_use",
				"port":    *req.HostPort,
				"used_by": existingName,
			})
			return
		}

		var proxyName string
		err = h.db.QueryRow("SELECT name FROM proxies WHERE host_port = ?", *req.HostPort).Scan(&proxyName)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "port_in_use",
				"port":    *req.HostPort,
				"used_by": proxyName,
			})
			return
		}
	}

	var existingID int
	err := h.db.QueryRow("SELECT id FROM servers WHERE name = ?", req.Name).Scan(&existingID)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "server name already exists"})
		return
	}

	if req.ProxyID != nil {
		var proxyExists bool
		err = h.db.QueryRow("SELECT 1 FROM proxies WHERE id = ?", *req.ProxyID).Scan(&proxyExists)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
			return
		}
	}

	validTypes := map[string]bool{
		"paper": true, "purpur": true, "fabric": true, "neoforge": true, "forge": true,
	}
	if !validTypes[strings.ToLower(req.Type)] {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid server type (must be paper, purpur, fabric, neoforge, or forge)",
		})
		return
	}

	var hostPortValue *int
	if req.ProxyID != nil {
		var serverCount int
		err = h.db.QueryRow("SELECT COUNT(*) FROM servers WHERE proxy_id = ?", *req.ProxyID).Scan(&serverCount)
		if err != nil {
			serverCount = 0
		}
		autoPort := 30000 + int(*req.ProxyID*100) + serverCount + 1
		hostPortValue = &autoPort
	} else if req.HostPort != nil {
		hostPortValue = req.HostPort
	}

	var minimotdLine1, minimotdLine2 interface{}
	if req.MinimotdLine1 != nil {
		minimotdLine1 = *req.MinimotdLine1
	} else {
		minimotdLine1 = nil
	}
	if req.MinimotdLine2 != nil {
		minimotdLine2 = *req.MinimotdLine2
	} else {
		minimotdLine2 = nil
	}

	result, err := h.db.Exec(`
		INSERT INTO servers (name, type, version, proxy_id, host_port, ram_mb, domain, backup_interval_days, auto_shutdown_minutes, scheduled_start, scheduled_stop, status, canvas_x, canvas_y, minimotd_line1, minimotd_line2)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'stopped', 4000, 4000, ?, ?)
	`, req.Name, req.Type, req.Version, req.ProxyID, hostPortValue, req.RAMMB, req.Domain, req.BackupIntervalDays, req.AutoShutdownMinutes, req.ScheduledStart, req.ScheduledStop, minimotdLine1, minimotdLine2)

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create server"})
		return
	}

	id, _ := result.LastInsertId()

	serverPath := filepath.Join(h.cfg.ServersDir, req.Name)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create server directory"})
		return
	}

	jarPath := filepath.Join(serverPath, "server.jar")
	jarBuild, jarHash, err := mc.DownloadServerJar(req.Type, req.Version, jarPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to download server jar: %v", err)})
		return
	}

	if req.Type == "paper" {
		if _, err := h.db.Exec("UPDATE servers SET jar_build = ? WHERE id = ?", jarBuild, id); err != nil {
			fmt.Printf("Failed to update jar_build for server %d: %v\n", id, err)
		}
	} else if req.Type == "purpur" {
		if _, err := h.db.Exec("UPDATE servers SET jar_hash = ? WHERE id = ?", jarHash, id); err != nil {
			fmt.Printf("Failed to update jar_hash for server %d: %v\n", id, err)
		}
	}

	if err := mc.WriteEntrypointScript(serverPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to write entrypoint script: %v", err)})
		return
	}
	props := mc.DefaultServerProperties()
	if hostPortValue != nil {
		props.Set("server-port", strconv.Itoa(*hostPortValue))
	}
	if req.ProxyID != nil {
		props.Set("online-mode", "false")
	}
	props.WriteToFile(filepath.Join(serverPath, "server.properties"))

	if err := os.WriteFile(filepath.Join(serverPath, "eula.txt"), []byte("eula=true\n"), 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create eula.txt"})
		return
	}

	if req.ProxyID != nil {
		var forwardingSecret string
		var proxyName string
		var proxyHostPort int
		err := h.db.QueryRow("SELECT forwarding_secret, name, host_port FROM proxies WHERE id = ?", *req.ProxyID).Scan(&forwardingSecret, &proxyName, &proxyHostPort)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to get forwarding secret"})
			return
		}
		if err := ConfigureServerProxy(serverPath, req.Type, req.Version, forwardingSecret); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to configure proxy: %v", err)})
			return
		}
		if err := SyncProxyConfig(h.db, *req.ProxyID, h.cfg.ServersDir); err != nil {
		}

		if req.MinimotdLine1 != nil && *req.MinimotdLine1 != "" || req.MinimotdLine2 != nil && *req.MinimotdLine2 != "" {
			line1 := ""
			line2 := ""
			if req.MinimotdLine1 != nil {
				line1 = *req.MinimotdLine1
			}
			if req.MinimotdLine2 != nil {
				line2 = *req.MinimotdLine2
			}

			iconPath := filepath.Join(serverPath, "server-icon.png")
			hasIcon := false
			if _, err := os.Stat(iconPath); err == nil {
				hasIcon = true
			}

			if err := h.minimotdMgr.CreateExtraConfig(proxyName, req.Name, line1, line2, hasIcon); err != nil {
				fmt.Printf("Failed to create MiniMOTD extra config: %v\n", err)
			}

			if hasIcon {
				iconData, err := os.ReadFile(iconPath)
				if err == nil {
					if err := h.minimotdMgr.CopyIcon(proxyName, req.Name, iconData); err != nil {
						fmt.Printf("Failed to copy icon to MiniMOTD: %v\n", err)
					}
				}
			}
		}

		if req.Domain != nil && *req.Domain != "" {
			if err := h.minimotdMgr.AddVirtualHost(proxyName, *req.Domain, fmt.Sprintf("%d", proxyHostPort), req.Name); err != nil {
				fmt.Printf("Failed to add virtual host to MiniMOTD: %v\n", err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

func (h *ServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var s models.ServerResponse
	var proxyID sql.NullInt64
	var proxyName sql.NullString
	var domain sql.NullString
	var scheduledStart sql.NullString
	var scheduledStop sql.NullString
	var hostPort sql.NullInt64
	var minimotdLine1 sql.NullString
	var minimotdLine2 sql.NullString

	err = h.db.QueryRow(`
		SELECT s.id, s.name, s.type, s.version, s.proxy_id, p.name, s.ram_mb, s.domain,
		       s.backup_interval_days, s.auto_shutdown_minutes, s.scheduled_start, s.scheduled_stop,
		       s.host_port, s.status, s.canvas_x, s.canvas_y, s.jar_build, s.created_at,
		       COALESCE((SELECT COUNT(*) FROM players WHERE server_id = s.id), 0) as player_count,
		       s.minimotd_line1, s.minimotd_line2
		FROM servers s
		LEFT JOIN proxies p ON s.proxy_id = p.id
		WHERE s.id = ?
	`, id).Scan(
		&s.ID, &s.Name, &s.Type, &s.Version, &proxyID, &proxyName, &s.RAMMB, &domain,
		&s.BackupIntervalDays, &s.AutoShutdownMinutes, &scheduledStart, &scheduledStop,
		&hostPort, &s.Status, &s.CanvasX, &s.CanvasY, &s.JarBuild, &s.CreatedAt, &s.PlayerCount,
		&minimotdLine1, &minimotdLine2,
	)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get server"})
		return
	}

	if proxyID.Valid {
		pid := proxyID.Int64
		s.ProxyID = &pid
	}
	s.ProxyName = nullStringToPtr(proxyName)
	s.Domain = nullStringToPtr(domain)
	s.ScheduledStart = nullStringToPtr(scheduledStart)
	s.ScheduledStop = nullStringToPtr(scheduledStop)
	if hostPort.Valid {
		hp := int(hostPort.Int64)
		s.HostPort = &hp
	}
	s.IconPath = h.getIconPath(s.ID, s.Name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	var status string
	var proxyID sql.NullInt64
	var domain sql.NullString
	err = h.db.QueryRow("SELECT name, status, proxy_id, domain FROM servers WHERE id = ?", id).Scan(&name, &status, &proxyID, &domain)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	ctx := context.Background()
	if status == "running" {
		timeout := 10
		_ = h.docker.StopContainer(ctx, name, &timeout)
	}

	if exists, _ := h.docker.ContainerExists(ctx, name); exists {
		_ = h.docker.RemoveContainer(ctx, name)
	}

	serverPath := filepath.Join(h.cfg.ServersDir, name)
	_ = os.RemoveAll(serverPath)

	result, err := h.db.Exec("DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete server"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	if proxyID.Valid {
		_ = SyncProxyConfig(h.db, proxyID.Int64, h.cfg.ServersDir)

		var proxyName string
		var proxyHostPort int
		if err := h.db.QueryRow("SELECT name, host_port FROM proxies WHERE id = ?", proxyID.Int64).Scan(&proxyName, &proxyHostPort); err == nil {
			_ = h.minimotdMgr.DeleteExtraConfig(proxyName, name)
			_ = h.minimotdMgr.DeleteIcon(proxyName, name)
			if domain.Valid {
				_ = h.minimotdMgr.RemoveVirtualHost(proxyName, domain.String, fmt.Sprintf("%d", proxyHostPort))
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

type UpdateServerRequest struct {
	Name                *string `json:"name"`
	RAMMB               *int    `json:"ram_mb"`
	AutoShutdownMinutes *int    `json:"auto_shutdown_minutes"`
	BackupIntervalDays  *int    `json:"backup_interval_days"`
	ScheduledStart      *string `json:"scheduled_start"`
	ScheduledStop       *string `json:"scheduled_stop"`
	CanvasX             *int    `json:"canvas_x"`
	CanvasY             *int    `json:"canvas_y"`
	ProxyID             *int64  `json:"proxy_id"`
	Domain              *string `json:"domain"`
	MinimotdLine1       *string `json:"minimotd_line1"`
	MinimotdLine2       *string `json:"minimotd_line2"`
}

func (h *ServerHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var req UpdateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	var exists bool
	err = h.db.QueryRow("SELECT 1 FROM servers WHERE id = ?", id).Scan(&exists)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	if req.Name != nil {
		h.db.Exec("UPDATE servers SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.Name, id)
	}
	if req.RAMMB != nil {
		h.db.Exec("UPDATE servers SET ram_mb = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.RAMMB, id)
	}
	if req.AutoShutdownMinutes != nil {
		h.db.Exec("UPDATE servers SET auto_shutdown_minutes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.AutoShutdownMinutes, id)
	}
	if req.BackupIntervalDays != nil {
		h.db.Exec("UPDATE servers SET backup_interval_days = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.BackupIntervalDays, id)
	}
	if req.ScheduledStart != nil {
		h.db.Exec("UPDATE servers SET scheduled_start = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.ScheduledStart, id)
	}
	if req.ScheduledStop != nil {
		h.db.Exec("UPDATE servers SET scheduled_stop = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.ScheduledStop, id)
	}
	if req.CanvasX != nil {
		h.db.Exec("UPDATE servers SET canvas_x = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.CanvasX, id)
	}
	if req.CanvasY != nil {
		h.db.Exec("UPDATE servers SET canvas_y = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.CanvasY, id)
	}
	if req.MinimotdLine1 != nil {
		h.db.Exec("UPDATE servers SET minimotd_line1 = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.MinimotdLine1, id)
	}
	if req.MinimotdLine2 != nil {
		h.db.Exec("UPDATE servers SET minimotd_line2 = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.MinimotdLine2, id)
	}

	var currentName sql.NullString
	var currentProxyID sql.NullInt64
	var currentDomain sql.NullString
	var proxyHostPort int
	var proxyName string
	h.db.QueryRow("SELECT name, proxy_id, domain FROM servers WHERE id = ?", id).Scan(&currentName, &currentProxyID, &currentDomain)

	if currentProxyID.Valid {
		h.db.QueryRow("SELECT name, host_port FROM proxies WHERE id = ?", currentProxyID.Int64).Scan(&proxyName, &proxyHostPort)
	}

	if req.Name != nil {
		h.db.Exec("UPDATE servers SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.Name, id)
	}
	if req.RAMMB != nil {
		h.db.Exec("UPDATE servers SET ram_mb = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.RAMMB, id)
	}
	if req.AutoShutdownMinutes != nil {
		h.db.Exec("UPDATE servers SET auto_shutdown_minutes = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.AutoShutdownMinutes, id)
	}
	if req.BackupIntervalDays != nil {
		h.db.Exec("UPDATE servers SET backup_interval_days = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.BackupIntervalDays, id)
	}
	if req.ScheduledStart != nil {
		h.db.Exec("UPDATE servers SET scheduled_start = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.ScheduledStart, id)
	}
	if req.ScheduledStop != nil {
		h.db.Exec("UPDATE servers SET scheduled_stop = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.ScheduledStop, id)
	}
	if req.CanvasX != nil {
		h.db.Exec("UPDATE servers SET canvas_x = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.CanvasX, id)
	}
	if req.CanvasY != nil {
		h.db.Exec("UPDATE servers SET canvas_y = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.CanvasY, id)
	}

	var hasConfig bool
	var minimotdLine1, minimotdLine2 sql.NullString
	h.db.QueryRow("SELECT minimotd_line1, minimotd_line2 FROM servers WHERE id = ?", id).Scan(&minimotdLine1, &minimotdLine2)
	hasConfig = minimotdLine1.Valid || minimotdLine2.Valid

	if req.ProxyID != nil {
		if *req.ProxyID == 0 {
			if currentProxyID.Valid && hasConfig {
				_ = h.minimotdMgr.DeleteExtraConfig(proxyName, currentName.String)
				if currentDomain.Valid {
					_ = h.minimotdMgr.RemoveVirtualHost(proxyName, currentDomain.String, fmt.Sprintf("%d", proxyHostPort))
				}
			}
			_ = RemoveServerFromProxy(h.db, id, h.cfg.ServersDir)
		} else {
			var newProxyName string
			var newProxyHostPort int
			err := h.db.QueryRow("SELECT name, host_port FROM proxies WHERE id = ?", *req.ProxyID).Scan(&newProxyName, &newProxyHostPort)
			if err == nil {
				if hasConfig {
					_ = h.minimotdMgr.DeleteExtraConfig(proxyName, currentName.String)
					_ = h.minimotdMgr.DeleteIcon(proxyName, currentName.String)
					if currentDomain.Valid {
						_ = h.minimotdMgr.RemoveVirtualHost(proxyName, currentDomain.String, fmt.Sprintf("%d", proxyHostPort))
					}

					line1 := ""
					line2 := ""
					if minimotdLine1.Valid {
						line1 = minimotdLine1.String
					}
					if minimotdLine2.Valid {
						line2 = minimotdLine2.String
					}

					serverPath := filepath.Join(h.cfg.ServersDir, currentName.String)
					iconPath := filepath.Join(serverPath, "server-icon.png")
					hasIcon := false
					if _, err := os.Stat(iconPath); err == nil {
						hasIcon = true
					}

					if err := h.minimotdMgr.CreateExtraConfig(newProxyName, currentName.String, line1, line2, hasIcon); err != nil {
						fmt.Printf("Failed to create MiniMOTD extra config: %v\n", err)
					}

					if hasIcon {
						iconData, err := os.ReadFile(iconPath)
						if err == nil {
							if err := h.minimotdMgr.CopyIcon(newProxyName, currentName.String, iconData); err != nil {
								fmt.Printf("Failed to copy icon to MiniMOTD: %v\n", err)
							}
						}
					}

					if currentDomain.Valid {
						if err := h.minimotdMgr.AddVirtualHost(newProxyName, currentDomain.String, fmt.Sprintf("%d", newProxyHostPort), currentName.String); err != nil {
							fmt.Printf("Failed to add virtual host to MiniMOTD: %v\n", err)
						}
					}
				}
			}
			_, _ = AssignServerToProxy(h.db, id, *req.ProxyID, h.cfg.ServersDir)
		}
	}
	if req.Domain != nil {
		h.db.Exec("UPDATE servers SET domain = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.Domain, id)
		var proxyID sql.NullInt64
		h.db.QueryRow("SELECT proxy_id FROM servers WHERE id = ?", id).Scan(&proxyID)
		if proxyID.Valid {
			_ = SyncProxyConfig(h.db, proxyID.Int64, h.cfg.ServersDir)

			if currentDomain.Valid {
				_ = h.minimotdMgr.RemoveVirtualHost(proxyName, currentDomain.String, fmt.Sprintf("%d", proxyHostPort))
			}

			if *req.Domain != "" && hasConfig {
				if err := h.minimotdMgr.AddVirtualHost(proxyName, *req.Domain, fmt.Sprintf("%d", proxyHostPort), currentName.String); err != nil {
					fmt.Printf("Failed to add virtual host to MiniMOTD: %v\n", err)
				}
			}
		}
	}

	if req.Name != nil && hasConfig && currentProxyID.Valid {
		domain := ""
		if currentDomain.Valid {
			domain = currentDomain.String
		}

		if err := h.minimotdMgr.RenameServer(proxyName, currentName.String, *req.Name, domain, fmt.Sprintf("%d", proxyHostPort)); err != nil {
			fmt.Printf("Failed to rename MiniMOTD config: %v\n", err)
		}
	}

	if (req.MinimotdLine1 != nil || req.MinimotdLine2 != nil) && currentProxyID.Valid {
		line1 := ""
		line2 := ""
		if req.MinimotdLine1 != nil {
			line1 = *req.MinimotdLine1
		} else if minimotdLine1.Valid {
			line1 = minimotdLine1.String
		}
		if req.MinimotdLine2 != nil {
			line2 = *req.MinimotdLine2
		} else if minimotdLine2.Valid {
			line2 = minimotdLine2.String
		}

		serverPath := filepath.Join(h.cfg.ServersDir, currentName.String)
		iconPath := filepath.Join(serverPath, "server-icon.png")
		hasIcon := false
		if _, err := os.Stat(iconPath); err == nil {
			hasIcon = true
		}

		if line1 != "" || line2 != "" {
			if err := h.minimotdMgr.UpdateExtraConfig(proxyName, currentName.String, line1, line2, hasIcon); err != nil {
				fmt.Printf("Failed to update MiniMOTD extra config: %v\n", err)
			}
		} else {
			_ = h.minimotdMgr.DeleteExtraConfig(proxyName, currentName.String)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) Start(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name, serverType, version string
	var ramMB int
	var hostPort sql.NullInt64
	err = h.db.QueryRow(`
		SELECT name, type, version, ram_mb, host_port FROM servers WHERE id = ?`, id).
		Scan(&name, &serverType, &version, &ramMB, &hostPort)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	port := 0
	if hostPort.Valid {
		port = int(hostPort.Int64)
	}

	cfg := &docker.ServerContainerConfig{
		Name:        name,
		ServerType:  serverType,
		Version:     version,
		RAMMB:       ramMB,
		ServerPath:  filepath.Join(h.cfg.HostServersDir, name),
		NetworkName: h.cfg.NetworkName,
		HostPort:    port,
	}

	ctx := context.Background()
	if err := h.docker.StartContainer(ctx, name, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to start server: %v", err)})
		return
	}

	h.db.Exec("UPDATE servers SET status = 'running', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Server %s started", name),
	})
}

func (h *ServerHandler) Stop(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	ctx := context.Background()
	timeout := 30
	if err := h.docker.StopContainer(ctx, name, &timeout); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to stop server: %v", err)})
		return
	}

	h.db.Exec("UPDATE servers SET status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) Restart(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name, serverType, version string
	var ramMB int
	var hostPort sql.NullInt64
	err = h.db.QueryRow(`
		SELECT name, type, version, ram_mb, host_port FROM servers WHERE id = ?`, id).
		Scan(&name, &serverType, &version, &ramMB, &hostPort)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	ctx := context.Background()
	timeout := 30
	if err := h.docker.StopContainer(ctx, name, &timeout); err != nil {
	}

	port := 0
	if hostPort.Valid {
		port = int(hostPort.Int64)
	}

	cfg := &docker.ServerContainerConfig{
		Name:        name,
		ServerType:  serverType,
		Version:     version,
		RAMMB:       ramMB,
		ServerPath:  filepath.Join(h.cfg.HostServersDir, name),
		NetworkName: h.cfg.NetworkName,
		HostPort:    port,
	}

	if err := h.docker.StartContainer(ctx, name, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to restart server: %v", err)})
		return
	}

	h.db.Exec("UPDATE servers SET status = 'running', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			lines = parsed
		}
	}

	ctx := context.Background()
	logs, err := h.docker.GetContainerLogs(ctx, name, lines)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get logs"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"logs": logs})
}

type CommandRequest struct {
	Command string `json:"command"`
}

func (h *ServerHandler) ExecuteCommand(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	var req CommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Command == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "command required"})
		return
	}

	if err := h.consoleManager.SendCommand(id, req.Command); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to execute command"})
		return
	}

	h.db.Exec("INSERT INTO command_history (server_id, command) VALUES (?, ?)", id, req.Command)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) GetCommandHistory(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	rows, err := h.db.Query(
		"SELECT command, executed_at FROM command_history WHERE server_id = ? ORDER BY executed_at DESC LIMIT ?",
		id, limit,
	)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get history"})
		return
	}
	defer rows.Close()

	var commands []map[string]string
	for rows.Next() {
		var cmd, executedAt string
		if err := rows.Scan(&cmd, &executedAt); err != nil {
			continue
		}
		commands = append(commands, map[string]string{
			"command":     cmd,
			"executed_at": executedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"commands": commands})
}

func (h *ServerHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		relativePath = "/"
	}

	relativePath = filepath.Clean("/" + relativePath)
	serverPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(serverPath, relativePath)

	if !strings.HasPrefix(fullPath, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "directory not found"})
		return
	}

	files := make([]map[string]interface{}, len(entries))
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, entry := range entries {
		wg.Add(1)
		go func(idx int, e fs.DirEntry) {
			defer wg.Done()

			name := e.Name()
			isDir := e.IsDir()
			var size int64
			var modified string

			if !isDir {
				if info, err := e.Info(); err == nil {
					size = info.Size()
					modified = info.ModTime().Format("2006-01-02T15:04:05Z")
				}
			}

			mu.Lock()
			files[idx] = map[string]interface{}{
				"name":     name,
				"is_dir":   isDir,
				"size":     size,
				"modified": modified,
			}
			mu.Unlock()
		}(i, entry)
	}
	wg.Wait()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":    relativePath,
		"entries": files,
	})
}

func (h *ServerHandler) GetFileContent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "path required"})
		return
	}

	relativePath = filepath.Clean("/" + relativePath)
	serverPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(serverPath, relativePath)

	if !strings.HasPrefix(fullPath, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	var content []byte
	isGzipped := strings.HasSuffix(fullPath, ".gz")

	if isGzipped {
		f, err := os.Open(fullPath)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "file not found"})
			return
		}
		defer f.Close()

		gzReader, err := gzip.NewReader(f)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "corrupted gzip file"})
			return
		}
		defer gzReader.Close()

		content, err = io.ReadAll(gzReader)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "failed to decompress file"})
			return
		}
	} else {
		content, err = os.ReadFile(fullPath)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "file not found"})
			return
		}
	}

	if !isTextFile(content) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "binary file"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	response := map[string]string{"content": string(content)}
	if isGzipped {
		response["is_gzipped"] = "true"
	}
	json.NewEncoder(w).Encode(response)
}

type FileContentRequest struct {
	Content string `json:"content"`
}

func (h *ServerHandler) WriteFileContent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "path required"})
		return
	}

	var req FileContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	relativePath = filepath.Clean("/" + relativePath)
	serverPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(serverPath, relativePath)

	if !strings.HasPrefix(fullPath, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to write file"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "path required"})
		return
	}

	relativePath = filepath.Clean("/" + relativePath)
	serverPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(serverPath, relativePath)

	if !strings.HasPrefix(fullPath, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	if err := os.RemoveAll(fullPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "path required"})
		return
	}

	relativePath = filepath.Clean("/" + relativePath)
	serverPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(serverPath, relativePath)

	if !strings.HasPrefix(fullPath, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	file, err := os.Open(fullPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "file not found"})
		return
	}
	defer file.Close()

	baseName := filepath.Base(fullPath)
	ext := strings.ToLower(filepath.Ext(baseName))

	contentType := "application/octet-stream"
	contentDisposition := "attachment"

	switch ext {
	case ".png":
		contentType = "image/png"
		contentDisposition = "inline"
	case ".jpg", ".jpeg":
		contentType = "image/jpeg"
		contentDisposition = "inline"
	case ".gif":
		contentType = "image/gif"
		contentDisposition = "inline"
	case ".webp":
		contentType = "image/webp"
		contentDisposition = "inline"
	case ".bmp":
		contentType = "image/bmp"
		contentDisposition = "inline"
	case ".ico":
		contentType = "image/x-icon"
		contentDisposition = "inline"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%s", contentDisposition, baseName))
	io.Copy(w, file)
}

type RenameRequest struct {
	OldPath string `json:"old_path"`
	NewName string `json:"new_name"`
}

func (h *ServerHandler) RenameFile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	var req RenameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.OldPath == "" || req.NewName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "old_path and new_name required"})
		return
	}

	serverPath := filepath.Join(h.cfg.ServersDir, name)
	oldPath := filepath.Join(serverPath, filepath.Clean("/"+req.OldPath))
	newPath := filepath.Join(filepath.Dir(oldPath), req.NewName)

	if !strings.HasPrefix(oldPath, serverPath) || !strings.HasPrefix(newPath, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to rename"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) UploadIcon(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	if err := r.ParseMultipartForm(2 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large (max 2MB)"})
		return
	}

	file, header, err := r.FormFile("icon")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "icon file required"})
		return
	}
	defer file.Close()

	if !strings.HasSuffix(strings.ToLower(header.Filename), ".png") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "icon must be a PNG file"})
		return
	}

	content, err := io.ReadAll(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read file"})
		return
	}

	if !isValidPNGIcon(content) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "icon must be a valid 64x64 PNG image"})
		return
	}

	serverPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(serverPath, "server-icon.png")

	if err := os.WriteFile(iconPath, content, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save icon"})
		return
	}

	var proxyID sql.NullInt64
	var minimotdLine1, minimotdLine2 sql.NullString
	h.db.QueryRow("SELECT proxy_id, minimotd_line1, minimotd_line2 FROM servers WHERE id = ?", id).Scan(&proxyID, &minimotdLine1, &minimotdLine2)

	if proxyID.Valid && (minimotdLine1.Valid || minimotdLine2.Valid) {
		var proxyName string
		if err := h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", proxyID.Int64).Scan(&proxyName); err == nil {
			if err := h.minimotdMgr.CopyIcon(proxyName, name, content); err != nil {
				fmt.Printf("Failed to copy icon to MiniMOTD: %v\n", err)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ServerHandler) GetIcon(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	serverPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(serverPath, "server-icon.png")

	content, err := os.ReadFile(iconPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "icon not found"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(content)
}

func (h *ServerHandler) DeleteIcon(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var name string
	var proxyID sql.NullInt64
	err = h.db.QueryRow("SELECT name, proxy_id FROM servers WHERE id = ?", id).Scan(&name, &proxyID)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	serverPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(serverPath, "server-icon.png")

	if _, err := os.Stat(iconPath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "icon not found"})
		return
	}

	if err := os.Remove(iconPath); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete icon"})
		return
	}

	if proxyID.Valid {
		var proxyName string
		if err := h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", proxyID.Int64).Scan(&proxyName); err == nil {
			_ = h.minimotdMgr.DeleteIcon(proxyName, name)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func isValidPNGIcon(content []byte) bool {
	if len(content) < 8 {
		return false
	}

	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	for i := 0; i < 8; i++ {
		if content[i] != pngHeader[i] {
			return false
		}
	}

	if len(content) < 33 {
		return false
	}

	width := int(content[16])<<24 | int(content[17])<<16 | int(content[18])<<8 | int(content[19])
	height := int(content[20])<<24 | int(content[21])<<16 | int(content[22])<<8 | int(content[23])

	return width == 64 && height == 64
}

func (h *ServerHandler) getIconPath(id int64, name string) *string {
	serverPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(serverPath, "server-icon.png")

	if _, err := os.Stat(iconPath); err == nil {
		path := fmt.Sprintf("/api/servers/%d/icon", id)
		return &path
	}
	return nil
}

func isTextFile(content []byte) bool {
	if len(content) == 0 {
		return true
	}

	nullCount := 0
	for i, b := range content {
		if b == 0 {
			nullCount++
		}
		if i > 8192 {
			break
		}
	}

	return nullCount == 0
}
