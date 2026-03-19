package handlers

import (
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/demimine/manager/internal/mc"
	"github.com/demimine/manager/internal/models"
	"github.com/demimine/manager/internal/plugin"
	"github.com/go-chi/chi/v5"
)

type ProxyHandler struct {
	db             *sql.DB
	docker         *docker.Client
	consoleManager *docker.ConsoleManager
	cfg            *config.Config
	pluginManager  *plugin.Manager
}

func NewProxyHandler(db *sql.DB, dockerClient *docker.Client, consoleManager *docker.ConsoleManager, cfg *config.Config, pluginManager *plugin.Manager) *ProxyHandler {
	return &ProxyHandler{
		db:             db,
		docker:         dockerClient,
		consoleManager: consoleManager,
		cfg:            cfg,
		pluginManager:  pluginManager,
	}
}

func (h *ProxyHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT p.id, p.name, p.host_port, p.ram_mb, p.forwarding_secret, p.status, p.canvas_x, p.canvas_y, p.jar_version, p.jar_build, p.created_at,
		       COALESCE(s.name, '') as server_name
		FROM proxies p
		LEFT JOIN servers s ON s.proxy_id = p.id
		ORDER BY p.created_at DESC, s.name
	`)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to list proxies"})
		return
	}
	defer rows.Close()

	proxyMap := make(map[int64]*models.ProxyResponse)
	var proxyOrder []int64

	for rows.Next() {
		var proxyID int64
		var name, status string
		var hostPort, ramMB int
		var forwardingSecret sql.NullString
		var canvasX, canvasY int
		var jarVersion sql.NullString
		var jarBuild int
		var createdAt string
		var serverName sql.NullString

		err := rows.Scan(
			&proxyID, &name, &hostPort, &ramMB, &forwardingSecret, &status, &canvasX, &canvasY, &jarVersion, &jarBuild, &createdAt, &serverName,
		)
		if err != nil {
			continue
		}

		proxy, exists := proxyMap[proxyID]
		if !exists {
			proxy = &models.ProxyResponse{
				ID:               proxyID,
				Name:             name,
				HostPort:         hostPort,
				RAMMB:            ramMB,
				Status:           status,
				CanvasX:          canvasX,
				CanvasY:          canvasY,
				JarBuild:         jarBuild,
				CreatedAt:        createdAt,
				ConnectedServers: []string{},
				IconPath:         h.getIconPath(proxyID, name),
			}
			if forwardingSecret.Valid {
				proxy.ForwardingSecret = forwardingSecret.String
			}
			if jarVersion.Valid {
				proxy.JarVersion = &jarVersion.String
			}
			proxyMap[proxyID] = proxy
			proxyOrder = append(proxyOrder, proxyID)
		}

		if serverName.Valid && serverName.String != "" {
			proxy.ConnectedServers = append(proxy.ConnectedServers, serverName.String)
		}
	}

	proxies := make([]models.ProxyResponse, 0, len(proxyOrder))
	for _, id := range proxyOrder {
		proxies = append(proxies, *proxyMap[id])
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(proxies)
}

type CreateProxyRequest struct {
	Name               string `json:"name"`
	HostPort           int    `json:"host_port"`
	RAMMB              int    `json:"ram_mb"`
	InstallDemiAuth    bool   `json:"install_demiauth"`
	InstallDemiDynamic bool   `json:"install_demidynamic"`
	InstallLuckPerms   bool   `json:"install_luckperms"`
}

func (h *ProxyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" || req.HostPort == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "name and host_port are required"})
		return
	}

	force := r.URL.Query().Get("force") == "true"

	if !force {
		var existingName string
		err := h.db.QueryRow("SELECT name FROM proxies WHERE host_port = ?", req.HostPort).Scan(&existingName)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "port_in_use",
				"port":    req.HostPort,
				"used_by": existingName,
			})
			return
		}

		err = h.db.QueryRow("SELECT name FROM servers WHERE host_port = ?", req.HostPort).Scan(&existingName)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":   "port_in_use",
				"port":    req.HostPort,
				"used_by": existingName,
			})
			return
		}
	}

	var existingID int
	err := h.db.QueryRow("SELECT id FROM proxies WHERE name = ?", req.Name).Scan(&existingID)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy name already exists"})
		return
	}

	forwardingSecret, err := mc.GenerateForwardingSecret()
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to generate forwarding secret"})
		return
	}

	ramMB := req.RAMMB
	if ramMB == 0 {
		ramMB = 512
	}

	result, err := h.db.Exec(`
		INSERT INTO proxies (name, host_port, ram_mb, forwarding_secret, status, canvas_x, canvas_y)
		VALUES (?, ?, ?, ?, 'stopped', 4000, 4000)
	`, req.Name, req.HostPort, ramMB, forwardingSecret)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create proxy"})
		return
	}

	id, _ := result.LastInsertId()

	proxyPath := filepath.Join(h.cfg.ServersDir, req.Name)
	if err := os.MkdirAll(proxyPath, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create proxy directory"})
		return
	}

	jarPath := filepath.Join(proxyPath, "velocity.jar")
	jarVersion, jarBuild, err := mc.DownloadVelocityJar(jarPath)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to download velocity jar: %v", err)})
		return
	}

	if _, err := h.db.Exec("UPDATE proxies SET jar_version = ?, jar_build = ? WHERE id = ?", jarVersion, jarBuild, id); err != nil {
		fmt.Printf("Failed to update jar_version/jar_build for proxy %d: %v\n", id, err)
	}

	if err := mc.WriteForwardingSecret(proxyPath, forwardingSecret); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to write forwarding secret"})
		return
	}

	velocityConfig := mc.DefaultVelocityConfig(req.HostPort, forwardingSecret)
	if err := velocityConfig.WriteToFile(filepath.Join(proxyPath, "velocity.toml")); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to write velocity.toml"})
		return
	}

	// Auto-install MiniMOTD for MOTD support (non-blocking)
	_, _ = h.pluginManager.Install(plugin.InstallOptions{
		TargetType: "proxy",
		TargetID:   id,
		ProjectID:  "minimotd",
		Loaders:    []string{"velocity"},
		PluginsDir: req.Name,
	})

	// Install DemiAuth if requested
	if req.InstallDemiAuth {
		pluginsDir := filepath.Join(h.cfg.ServersDir, req.Name, "plugins")
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			fmt.Printf("Failed to create plugins directory for proxy %d: %v\n", id, err)
		}

		demiauthJar := filepath.Join(pluginsDir, "DemiAuth-1.0.0.jar")
		demiauthResource := "/app/resources/DemiAuth-1.0.0.jar"

		srcFile, err := os.Open(demiauthResource)
		if err != nil {
			fmt.Printf("Failed to open DemiAuth resource for proxy %d: %v\n", id, err)
		} else {
			defer srcFile.Close()
			destFile, err := os.Create(demiauthJar)
			if err != nil {
				fmt.Printf("Failed to create DemiAuth file for proxy %d: %v\n", id, err)
			} else {
				_, err := io.Copy(destFile, srcFile)
				destFile.Close()
				if err != nil {
					fmt.Printf("Failed to copy DemiAuth for proxy %d: %v\n", id, err)
				} else {
					fmt.Printf("Installed DemiAuth for proxy %d\n", id)
				}
			}
		}

		if err := configureDemiAuth(proxyPath, req.Name); err != nil {
			fmt.Printf("Failed to configure DemiAuth for proxy %d: %v\n", id, err)
		}
	}

	// Install DemiDynamic if requested
	if req.InstallDemiDynamic {
		pluginsDir := filepath.Join(h.cfg.ServersDir, req.Name, "plugins")
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			fmt.Printf("Failed to create plugins directory for proxy %d: %v\n", id, err)
		}

		demidynamicJar := filepath.Join(pluginsDir, "DemiDynamic-1.0.2.jar")
		demidynamicResource := "/app/resources/DemiDynamic-1.0.2.jar"

		srcFile, err := os.Open(demidynamicResource)
		if err != nil {
			fmt.Printf("Failed to open DemiDynamic resource for proxy %d: %v\n", id, err)
		} else {
			defer srcFile.Close()
			destFile, err := os.Create(demidynamicJar)
			if err != nil {
				fmt.Printf("Failed to create DemiDynamic file for proxy %d: %v\n", id, err)
			} else {
				_, err := io.Copy(destFile, srcFile)
				destFile.Close()
				if err != nil {
					fmt.Printf("Failed to copy DemiDynamic for proxy %d: %v\n", id, err)
				} else {
					fmt.Printf("Installed DemiDynamic for proxy %d\n", id)
				}
			}
		}
	}

	// Install LuckPerms if requested (via Modrinth API for correct Velocity version)
	if req.InstallLuckPerms {
		_, err := h.pluginManager.Install(plugin.InstallOptions{
			TargetType: "proxy",
			TargetID:   id,
			ProjectID:  "Vebnzrzj",
			Loaders:    []string{"velocity"},
			PluginsDir: req.Name,
		})
		if err != nil {
			fmt.Printf("Failed to install LuckPerms for proxy %d: %v\n", id, err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

func (h *ProxyHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var p models.ProxyResponse
	var forwardingSecret sql.NullString
	var jarVersion sql.NullString

	err = h.db.QueryRow(`
		SELECT id, name, host_port, ram_mb, forwarding_secret, status, canvas_x, canvas_y, jar_version, jar_build, created_at
		FROM proxies WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &p.HostPort, &p.RAMMB, &forwardingSecret, &p.Status, &p.CanvasX, &p.CanvasY, &jarVersion, &p.JarBuild, &p.CreatedAt)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get proxy"})
		return
	}

	if forwardingSecret.Valid {
		p.ForwardingSecret = forwardingSecret.String
	}

	if jarVersion.Valid {
		p.JarVersion = &jarVersion.String
	}

	serverRows, err := h.db.Query("SELECT name FROM servers WHERE proxy_id = ?", p.ID)
	if err == nil {
		for serverRows.Next() {
			var serverName string
			if err := serverRows.Scan(&serverName); err == nil {
				p.ConnectedServers = append(p.ConnectedServers, serverName)
			}
		}
		serverRows.Close()
	}

	p.IconPath = h.getIconPath(p.ID, p.Name)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(p)
}

func (h *ProxyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	var status string
	err = h.db.QueryRow("SELECT name, status FROM proxies WHERE id = ?", id).Scan(&name, &status)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	ctx := context.Background()
	if status == "running" {
		timeout := 10
		_ = h.docker.StopProxyContainer(ctx, name, &timeout)
	}

	containerName := "demimine-proxy-" + strings.ToLower(strings.ReplaceAll(name, " ", "-"))
	if exists, _ := h.docker.ContainerExists(ctx, "proxy-"+name); exists {
		_ = h.docker.RemoveContainer(ctx, containerName)
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	_ = os.RemoveAll(proxyPath)

	h.db.Exec("UPDATE servers SET proxy_id = NULL WHERE proxy_id = ?", id)

	result, err := h.db.Exec("DELETE FROM proxies WHERE id = ?", id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete proxy"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

type UpdateProxyRequest struct {
	Name    *string `json:"name"`
	RAMMB   *int    `json:"ram_mb"`
	CanvasX *int    `json:"canvas_x"`
	CanvasY *int    `json:"canvas_y"`
}

func (h *ProxyHandler) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var req UpdateProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	var exists bool
	err = h.db.QueryRow("SELECT 1 FROM proxies WHERE id = ?", id).Scan(&exists)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	if req.Name != nil {
		h.db.Exec("UPDATE proxies SET name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.Name, id)
	}
	if req.RAMMB != nil {
		h.db.Exec("UPDATE proxies SET ram_mb = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.RAMMB, id)
	}
	if req.CanvasX != nil {
		h.db.Exec("UPDATE proxies SET canvas_x = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.CanvasX, id)
	}
	if req.CanvasY != nil {
		h.db.Exec("UPDATE proxies SET canvas_y = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.CanvasY, id)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ProxyHandler) Start(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	var hostPort, ramMB int
	err = h.db.QueryRow("SELECT name, host_port, COALESCE(ram_mb, 512) FROM proxies WHERE id = ?", id).Scan(&name, &hostPort, &ramMB)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	cfg := &docker.ProxyContainerConfig{
		Name:        name,
		HostPort:    hostPort,
		ProxyPath:   filepath.Join(h.cfg.HostServersDir, name),
		NetworkName: h.cfg.NetworkName,
		RAMMB:       ramMB,
	}

	ctx := context.Background()
	if err := h.docker.StartProxyContainer(ctx, name, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to start proxy: %v", err)})
		return
	}

	h.db.Exec("UPDATE proxies SET status = 'running', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Proxy %s started", name),
	})
}

func (h *ProxyHandler) Stop(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	ctx := context.Background()
	timeout := 30
	if err := h.docker.StopProxyContainer(ctx, name, &timeout); err != nil {
		if strings.Contains(err.Error(), "No such container") {
			h.db.Exec("UPDATE proxies SET status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]bool{"success": true})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to stop proxy: %v", err)})
		return
	}

	h.db.Exec("UPDATE proxies SET status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ProxyHandler) Restart(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	var hostPort, ramMB int
	err = h.db.QueryRow("SELECT name, host_port, COALESCE(ram_mb, 512) FROM proxies WHERE id = ?", id).Scan(&name, &hostPort, &ramMB)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	ctx := context.Background()
	timeout := 30

	if err := h.docker.StopProxyContainer(ctx, name, &timeout); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to stop proxy: %v", err)})
		return
	}

	h.db.Exec("UPDATE proxies SET status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	containerID, err := h.docker.GetProxyContainerID(ctx, name)
	if err == nil && containerID != "" {
		if err := h.docker.WaitForContainer(ctx, containerID, 45*time.Second); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to wait for container to stop: %v", err)})
			return
		}
	}

	cfg := &docker.ProxyContainerConfig{
		Name:        name,
		HostPort:    hostPort,
		ProxyPath:   filepath.Join(h.cfg.HostServersDir, name),
		NetworkName: h.cfg.NetworkName,
		RAMMB:       ramMB,
	}

	if err := h.docker.StartProxyContainer(ctx, name, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to restart proxy: %v", err)})
		return
	}

	h.db.Exec("UPDATE proxies SET status = 'running', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ProxyHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	lines := 100
	if l := r.URL.Query().Get("lines"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			lines = parsed
		}
	}

	ctx := context.Background()
	logs, err := h.docker.GetProxyContainerLogs(ctx, name, lines)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get logs"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"logs": logs})
}

func (h *ProxyHandler) ListFiles(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		relativePath = "/"
	}

	relativePath = filepath.Clean("/" + relativePath)
	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(proxyPath, relativePath)

	if !strings.HasPrefix(fullPath, proxyPath) {
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
	for i, entry := range entries {
		name := entry.Name()
		isDir := entry.IsDir()
		var size int64
		var modified string

		if !isDir {
			if info, err := entry.Info(); err == nil {
				size = info.Size()
				modified = info.ModTime().Format("2006-01-02T15:04:05Z")
			}
		}

		files[i] = map[string]interface{}{
			"name":     name,
			"is_dir":   isDir,
			"size":     size,
			"modified": modified,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"path":    relativePath,
		"entries": files,
	})
}

func (h *ProxyHandler) GetFileContent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
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
	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(proxyPath, relativePath)

	if !strings.HasPrefix(fullPath, proxyPath) {
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

func (h *ProxyHandler) WriteFileContent(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
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
	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(proxyPath, relativePath)

	if !strings.HasPrefix(fullPath, proxyPath) {
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

func (h *ProxyHandler) DeleteFile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
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
	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(proxyPath, relativePath)

	if !strings.HasPrefix(fullPath, proxyPath) {
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

func (h *ProxyHandler) DownloadFile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
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
	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	fullPath := filepath.Join(proxyPath, relativePath)

	if !strings.HasPrefix(fullPath, proxyPath) {
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

func (h *ProxyHandler) RenameFile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
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

	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	oldPath := filepath.Join(proxyPath, filepath.Clean("/"+req.OldPath))
	newPath := filepath.Join(filepath.Dir(oldPath), req.NewName)

	if !strings.HasPrefix(oldPath, proxyPath) || !strings.HasPrefix(newPath, proxyPath) {
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

type ProxyCommandRequest struct {
	Command string `json:"command"`
}

func (h *ProxyHandler) ExecuteCommand(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var req ProxyCommandRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.Command == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "command is required"})
		return
	}

	var name string
	var status string
	err = h.db.QueryRow("SELECT name, status FROM proxies WHERE id = ?", id).Scan(&name, &status)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get proxy"})
		return
	}

	if status != "running" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy is not running"})
		return
	}

	if err := h.consoleManager.SendProxyCommand(id, req.Command); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to send command: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ProxyHandler) getIconPath(id int64, name string) *string {
	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(proxyPath, "icon.png")

	if _, err := os.Stat(iconPath); err == nil {
		path := fmt.Sprintf("/api/proxies/%d/icon", id)
		return &path
	}
	return nil
}

func (h *ProxyHandler) UploadIcon(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
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

	if !h.isValidProxyIcon(content) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "icon must be a valid 64x64 PNG image"})
		return
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(proxyPath, "icon.png")

	if err := os.WriteFile(iconPath, content, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save icon"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ProxyHandler) GetIcon(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(proxyPath, "icon.png")

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

func (h *ProxyHandler) DeleteIcon(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid proxy id"})
		return
	}

	var name string
	err = h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", id).Scan(&name)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, name)
	iconPath := filepath.Join(proxyPath, "icon.png")

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *ProxyHandler) isValidProxyIcon(content []byte) bool {
	if len(content) < 8 {
		return false
	}

	reader := strings.NewReader(string(content))
	img, _, err := image.DecodeConfig(reader)
	if err != nil {
		return false
	}

	return img.Width == 64 && img.Height == 64
}

func downloadPluginFile(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func configureDemiAuth(proxyPath, proxyName string) error {
	configPath := filepath.Join(proxyPath, "plugins", "demiauth", "config.toml")
	if err := os.MkdirAll(filepath.Join(proxyPath, "plugins", "demiauth"), 0755); err != nil {
		return fmt.Errorf("failed to create demiauth config directory: %w", err)
	}

	configContent := `#
# demiauth Configuration for Velocity Proxy
# Players must type in password in chat to authenticate

[core]
	# The server name set in Velocity that is used for authentication
	loginServer = "login"
	
	# The server name set in Velocity that player will be transferred to after logging in
	hubServer = "survival"
	
	# The password(s) players must type in chat to log in
	# Multiple passwords can be set. Add more passwords by adding them to the list like ["1234", "password"]
	serverPassword = ["changeme"]
	
	# Change to false to make players have to write password on every connection
	oneTimeLogin = true

[core.bypass]
	# If set to true, plugin will grant bypass permission to players automatically. Requires LuckPerms.
	# If you don't use LuckPerms, you must set bypass permissions manually
	pluginGrantsBypass = true
	
	# The permission node to check if a player has bypass permissions
	# Must exist either on the user or on a group the user is in
	bypassNode = "demimine.authenticated"
	
	# If a player with bypass permission transfers to the login server, how should they be handled?
	# Options: auto - automatically transfer them to the hub server
	#          deny-entry - prevent them from joining the login server
	bypasserLoginExitMethod = "auto"

[core.bypass.methods]
	# Method to grant bypass permissions when oneTimeLogin is enabled
	# "user" will grant bypassNode to the player
	# "group" will add the player to the bypassGroup (group must exist in permissions plugin)
	bypassMethod = "user"
	bypassGroup = "default"

[core.kick]
	# The amount of time to wait before kicking the player (in seconds)
	kickTimeout = 120
	
	# Message sent to player if they fail to authenticate within kickTimeout
	kickMessage = "Too many failed attempts"

[messages]
	# Message shown when player enters wrong password
	wrongPassword = "Wrong password. Please try again."
	
	# Message shown when player joins the login server
	# Accepts MiniMessage formatting: https://docs.papermc.io/adventure/minimessage/format/
	welcomeMessage = "<gradient:#ff6b6b:#feca57><b>Type your password in chat to continue</b></gradient>"

[misc]
	# Do not change this. Used for config migrations.
	configVersion = 1
	
	# Set to false to disable the plugin without removing it
	pluginEnabled = true
	
	# Set to true to enable debug logging
	debugMode = false
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return fmt.Errorf("failed to write DemiAuth config: %w", err)
	}

	return nil
}

func updateDemiAuthLoginServer(proxyPath, loginServerName string) error {
	configPath := filepath.Join(proxyPath, "plugins", "demiauth", "config.toml")

	content, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read demiauth config: %w", err)
	}

	configStr := string(content)
	re := regexp.MustCompile(`loginServer\s*=\s*"[^"]*"`)
	configStr = re.ReplaceAllString(configStr, fmt.Sprintf(`loginServer = "%s"`, loginServerName))

	if err := os.WriteFile(configPath, []byte(configStr), 0644); err != nil {
		return fmt.Errorf("failed to write demiauth config: %w", err)
	}

	return nil
}
