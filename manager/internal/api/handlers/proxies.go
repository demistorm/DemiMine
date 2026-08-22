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
	"log"
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
	"github.com/demimine/manager/internal/minimotd"
	"github.com/demimine/manager/internal/models"
	"github.com/demimine/manager/internal/plugin"
	"github.com/demimine/manager/internal/spark"
	"github.com/demimine/manager/internal/util"
	"github.com/go-chi/chi/v5"
)

type ProxyHandler struct {
	db             *sql.DB
	docker         *docker.Client
	consoleManager *docker.ConsoleManager
	cfg            *config.Config
	pluginManager  *plugin.Manager
	minimotdMgr    *minimotd.Manager
	sparkInstaller *spark.Installer
}

func NewProxyHandler(db *sql.DB, dockerClient *docker.Client, consoleManager *docker.ConsoleManager, cfg *config.Config, pluginManager *plugin.Manager, minimotdMgr *minimotd.Manager, sparkInstaller *spark.Installer) *ProxyHandler {
	return &ProxyHandler{
		db:             db,
		docker:         dockerClient,
		consoleManager: consoleManager,
		cfg:            cfg,
		pluginManager:  pluginManager,
		minimotdMgr:    minimotdMgr,
		sparkInstaller: sparkInstaller,
	}
}

func (h *ProxyHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT p.id, p.name, p.sanitized_name, p.host_port, p.ram_mb, p.forwarding_secret, p.status, p.canvas_x, p.canvas_y, p.motd_line1, p.motd_line2, p.jar_version, p.jar_build, p.created_at, p.start_on_boot, p.scheduled_start, p.scheduled_stop, p.jvm_flags, p.udp_port,
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
		var name, sanitizedName, status string
		var hostPort, ramMB, startOnBoot int
		var forwardingSecret sql.NullString
		var canvasX, canvasY int
		var motdLine1 sql.NullString
		var motdLine2 sql.NullString
		var jarVersion sql.NullString
		var jarBuild int
		var createdAt string
		var scheduledStart sql.NullString
		var scheduledStop sql.NullString
		var serverName sql.NullString
		var jvmFlags sql.NullString
		var udpPort sql.NullInt64

		err := rows.Scan(
			&proxyID, &name, &sanitizedName, &hostPort, &ramMB, &forwardingSecret, &status, &canvasX, &canvasY, &motdLine1, &motdLine2, &jarVersion, &jarBuild, &createdAt, &startOnBoot, &scheduledStart, &scheduledStop, &jvmFlags, &udpPort, &serverName,
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
				StartOnBoot:      startOnBoot,
				JarBuild:         jarBuild,
				CreatedAt:        createdAt,
				ConnectedServers: []string{},
				IconPath:         h.getIconPath(proxyID, sanitizedName),
			}
			if forwardingSecret.Valid {
				proxy.ForwardingSecret = forwardingSecret.String
			}
			if jarVersion.Valid {
				proxy.JarVersion = &jarVersion.String
			}
			if motdLine1.Valid {
				proxy.MotdLine1 = &motdLine1.String
			}
			if motdLine2.Valid {
				proxy.MotdLine2 = &motdLine2.String
			}
			if scheduledStart.Valid {
				proxy.ScheduledStart = &scheduledStart.String
			}
			if scheduledStop.Valid {
				proxy.ScheduledStop = &scheduledStop.String
			}
			if jvmFlags.Valid {
				proxy.JVMFlags = &jvmFlags.String
			}
			if udpPort.Valid {
				up := int(udpPort.Int64)
				proxy.UDPPort = &up
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
	MotdLine1          string `json:"motd_line1"`
	MotdLine2          string `json:"motd_line2"`
	StartOnBoot        int    `json:"start_on_boot"`
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

	sanitizedName := util.SanitizeName(req.Name)

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
	err := h.db.QueryRow("SELECT id FROM proxies WHERE sanitized_name = ?", sanitizedName).Scan(&existingID)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy name already exists"})
		return
	}

	var existingServerID int
	err = h.db.QueryRow("SELECT id FROM servers WHERE sanitized_name = ?", sanitizedName).Scan(&existingServerID)
	if err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{"error": "a server with this name already exists"})
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
		INSERT INTO proxies (name, sanitized_name, host_port, ram_mb, forwarding_secret, status, canvas_x, canvas_y, motd_line1, motd_line2, start_on_boot)
		VALUES (?, ?, ?, ?, ?, 'stopped', 4000, 4000, ?, ?, ?)
	`, req.Name, sanitizedName, req.HostPort, ramMB, forwardingSecret, req.MotdLine1, req.MotdLine2, req.StartOnBoot)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create proxy"})
		return
	}

	id, _ := result.LastInsertId()

	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	defaultFlags := mc.GetDefaultProxyJVMFlags()
	if _, err := h.db.Exec("UPDATE proxies SET jvm_flags = ? WHERE id = ?", defaultFlags, id); err != nil {
		fmt.Printf("Failed to set default JVM flags for proxy %d: %v\n", id, err)
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
		TargetType:   "proxy",
		TargetID:     id,
		ProjectID:    "minimotd",
		Loaders:      []string{"velocity"},
		ServerName:   sanitizedName,
		TargetSubdir: "plugins",
	})

	// Install DemiAuth if requested
	if req.InstallDemiAuth {
		pluginsDir := filepath.Join(proxyPath, "plugins")
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			fmt.Printf("Failed to create plugins directory for proxy %d: %v\n", id, err)
		}

		demiauthJar := filepath.Join(pluginsDir, "DemiAuth.jar")
		demiauthResource := "/app/resources/DemiAuth.jar"

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
		pluginsDir := filepath.Join(proxyPath, "plugins")
		if err := os.MkdirAll(pluginsDir, 0755); err != nil {
			fmt.Printf("Failed to create plugins directory for proxy %d: %v\n", id, err)
		}

		demidynamicJar := filepath.Join(pluginsDir, "DemiDynamic.jar")
		demidynamicResource := "/app/resources/DemiDynamic.jar"

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
			TargetType:   "proxy",
			TargetID:     id,
			ProjectID:    "Vebnzrzj",
			Loaders:      []string{"velocity"},
			ServerName:   sanitizedName,
			TargetSubdir: "plugins",
		})
		if err != nil {
			fmt.Printf("Failed to install LuckPerms for proxy %d: %v\n", id, err)
		}
	}

	if h.sparkInstaller != nil {
		if err := h.sparkInstaller.InstallSparkForProxy(req.Name, id); err != nil {
			log.Printf("Failed to install Spark for proxy %s: %v", req.Name, err)
		}
	}

	if req.MotdLine1 != "" || req.MotdLine2 != "" {
		iconPath := filepath.Join(proxyPath, "icon.png")
		hasIcon := false
		if _, err := os.Stat(iconPath); err == nil {
			hasIcon = true
			if err := h.minimotdMgr.CopyMainConfigIcon(sanitizedName, nil); err != nil {
				fmt.Printf("Failed to copy icon to MiniMOTD for proxy %d: %v\n", id, err)
			} else {
				if iconData, err := os.ReadFile(iconPath); err == nil {
					if err := h.minimotdMgr.CopyMainConfigIcon(sanitizedName, iconData); err != nil {
						fmt.Printf("Failed to copy icon to MiniMOTD for proxy %d: %v\n", id, err)
					}
				}
			}
		}
		if err := h.minimotdMgr.CreateMainConfig(sanitizedName, req.MotdLine1, req.MotdLine2, hasIcon); err != nil {
			fmt.Printf("Failed to create MiniMOTD main config for proxy %d: %v\n", id, err)
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
	var scheduledStart sql.NullString
	var scheduledStop sql.NullString
	var motdLine1 sql.NullString
	var motdLine2 sql.NullString
	var jvmFlags sql.NullString
	var udpPort sql.NullInt64
	var sanitizedName string

	err = h.db.QueryRow(`
		SELECT id, name, sanitized_name, host_port, ram_mb, forwarding_secret, status, canvas_x, canvas_y, motd_line1, motd_line2, jar_version, jar_build, created_at, start_on_boot, scheduled_start, scheduled_stop, jvm_flags, udp_port
		FROM proxies WHERE id = ?
	`, id).Scan(&p.ID, &p.Name, &sanitizedName, &p.HostPort, &p.RAMMB, &forwardingSecret, &p.Status, &p.CanvasX, &p.CanvasY, &motdLine1, &motdLine2, &jarVersion, &p.JarBuild, &p.CreatedAt, &p.StartOnBoot, &scheduledStart, &scheduledStop, &jvmFlags, &udpPort)

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

	if motdLine1.Valid {
		p.MotdLine1 = &motdLine1.String
	}

	if motdLine2.Valid {
		p.MotdLine2 = &motdLine2.String
	}

	if scheduledStart.Valid {
		p.ScheduledStart = &scheduledStart.String
	}

	if scheduledStop.Valid {
		p.ScheduledStop = &scheduledStop.String
	}

	if jvmFlags.Valid {
		p.JVMFlags = &jvmFlags.String
	}

	if udpPort.Valid {
		up := int(udpPort.Int64)
		p.UDPPort = &up
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

	p.IconPath = h.getIconPath(p.ID, sanitizedName)

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

	var sanitizedName string
	var status string
	err = h.db.QueryRow("SELECT sanitized_name, status FROM proxies WHERE id = ?", id).Scan(&sanitizedName, &status)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	ctx := context.Background()
	if status == "running" {
		timeout := 10
		_ = h.docker.StopProxyContainer(ctx, sanitizedName, &timeout)
	}

	containerName := "demimine-proxy-" + sanitizedName
	if exists, _ := h.docker.ContainerExists(ctx, "proxy-"+sanitizedName); exists {
		_ = h.docker.RemoveContainer(ctx, containerName)
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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
	Name           *string `json:"name"`
	RAMMB          *int    `json:"ram_mb"`
	CanvasX        *int    `json:"canvas_x"`
	CanvasY        *int    `json:"canvas_y"`
	MotdLine1      *string `json:"motd_line1"`
	MotdLine2      *string `json:"motd_line2"`
	JVMFlags       *string `json:"jvm_flags"`
	StartOnBoot    *int    `json:"start_on_boot"`
	ScheduledStart *string `json:"scheduled_start"`
	ScheduledStop  *string `json:"scheduled_stop"`
	UDPPort        *int    `json:"udp_port"`
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
	var currentSanitizedName string
	err = h.db.QueryRow("SELECT 1, COALESCE(sanitized_name, '') FROM proxies WHERE id = ?", id).Scan(&exists, &currentSanitizedName)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	if req.Name != nil {
		newSanitizedName := util.SanitizeName(*req.Name)

		var existingID int
		err := h.db.QueryRow("SELECT id FROM proxies WHERE sanitized_name = ? AND id != ?", newSanitizedName, id).Scan(&existingID)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "a proxy with a similar name already exists"})
			return
		}

		var existingServerID int
		err = h.db.QueryRow("SELECT id FROM servers WHERE sanitized_name = ?", newSanitizedName).Scan(&existingServerID)
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "a server with a similar name already exists"})
			return
		}

		if newSanitizedName != currentSanitizedName {
			oldPath := filepath.Join(h.cfg.ServersDir, currentSanitizedName)
			newPath := filepath.Join(h.cfg.ServersDir, newSanitizedName)

			if _, err := os.Stat(oldPath); err == nil {
				if _, err := os.Stat(newPath); err == nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					json.NewEncoder(w).Encode(map[string]string{"error": "a proxy directory with this name already exists"})
					return
				}
				if err := os.Rename(oldPath, newPath); err != nil {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to rename proxy directory: %v", err)})
					return
				}
			}
		}

		h.db.Exec("UPDATE proxies SET name = ?, sanitized_name = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.Name, newSanitizedName, id)
		currentSanitizedName = newSanitizedName
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
	if req.StartOnBoot != nil {
		h.db.Exec("UPDATE proxies SET start_on_boot = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.StartOnBoot, id)
	}
	if req.ScheduledStart != nil {
		h.db.Exec("UPDATE proxies SET scheduled_start = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.ScheduledStart, id)
	}
	if req.ScheduledStop != nil {
		h.db.Exec("UPDATE proxies SET scheduled_stop = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.ScheduledStop, id)
	}
	if req.JVMFlags != nil {
		h.db.Exec("UPDATE proxies SET jvm_flags = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.JVMFlags, id)
	}
	if req.UDPPort != nil {
		if *req.UDPPort > 0 {
			var conflictingName string
			err := h.db.QueryRow("SELECT name FROM proxies WHERE udp_port = ? AND id != ?", *req.UDPPort, id).Scan(&conflictingName)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "udp_port_in_use",
					"port":    *req.UDPPort,
					"used_by": conflictingName,
				})
				return
			}
			err = h.db.QueryRow("SELECT name FROM servers WHERE udp_port = ?", *req.UDPPort).Scan(&conflictingName)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "udp_port_in_use",
					"port":    *req.UDPPort,
					"used_by": conflictingName,
				})
				return
			}
			err = h.db.QueryRow("SELECT name FROM proxies WHERE host_port = ?", *req.UDPPort).Scan(&conflictingName)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "udp_port_in_use",
					"port":    *req.UDPPort,
					"used_by": conflictingName,
				})
				return
			}
			err = h.db.QueryRow("SELECT name FROM servers WHERE host_port = ?", *req.UDPPort).Scan(&conflictingName)
			if err == nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"error":   "udp_port_in_use",
					"port":    *req.UDPPort,
					"used_by": conflictingName,
				})
				return
			}
			h.db.Exec("UPDATE proxies SET udp_port = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", *req.UDPPort, id)
		} else {
			h.db.Exec("UPDATE proxies SET udp_port = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)
		}
	}

	if req.MotdLine1 != nil || req.MotdLine2 != nil {
		var name string
		var currentLine1 sql.NullString
		var currentLine2 sql.NullString
		h.db.QueryRow("SELECT name, motd_line1, motd_line2 FROM proxies WHERE id = ?", id).Scan(&name, &currentLine1, &currentLine2)

		newLine1 := ""
		newLine2 := ""
		if req.MotdLine1 != nil {
			newLine1 = *req.MotdLine1
		} else if currentLine1.Valid {
			newLine1 = currentLine1.String
		}
		if req.MotdLine2 != nil {
			newLine2 = *req.MotdLine2
		} else if currentLine2.Valid {
			newLine2 = currentLine2.String
		}

		if req.MotdLine1 != nil {
			h.db.Exec("UPDATE proxies SET motd_line1 = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newLine1, id)
		}
		if req.MotdLine2 != nil {
			h.db.Exec("UPDATE proxies SET motd_line2 = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", newLine2, id)
		}

		proxyIconPath := filepath.Join(h.cfg.ServersDir, currentSanitizedName, "icon.png")
		hasIcon := false
		if _, err := os.Stat(proxyIconPath); err == nil {
			hasIcon = true
			if err := h.minimotdMgr.CopyMainConfigIcon(currentSanitizedName, nil); err != nil {
				fmt.Printf("Failed to copy icon to MiniMOTD for proxy %d: %v\n", id, err)
			} else {
				if iconData, err := os.ReadFile(proxyIconPath); err == nil {
					if err := h.minimotdMgr.CopyMainConfigIcon(currentSanitizedName, iconData); err != nil {
						fmt.Printf("Failed to copy icon to MiniMOTD for proxy %d: %v\n", id, err)
					}
				}
			}
		}
		if err := h.minimotdMgr.CreateMainConfig(currentSanitizedName, newLine1, newLine2, hasIcon); err != nil {
			fmt.Printf("Failed to update MiniMOTD main config for proxy %d: %v\n", id, err)
		}
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
	var sanitizedName string
	var hostPort, ramMB int
	var jvmFlags sql.NullString
	var udpPort sql.NullInt64
	var jarVersion sql.NullString
	err = h.db.QueryRow("SELECT name, sanitized_name, host_port, COALESCE(ram_mb, 512), jvm_flags, udp_port, jar_version FROM proxies WHERE id = ?", id).Scan(&name, &sanitizedName, &hostPort, &ramMB, &jvmFlags, &udpPort, &jarVersion)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	jvmFlagsStr := ""
	if jvmFlags.Valid {
		jvmFlagsStr = jvmFlags.String
	}

	udpPortVal := 0
	if udpPort.Valid {
		udpPortVal = int(udpPort.Int64)
	}

	cfg := &docker.ProxyContainerConfig{
		Name:        sanitizedName,
		HostPort:    hostPort,
		UDPPort:     udpPortVal,
		ProxyPath:   filepath.Join(h.cfg.HostServersDir, sanitizedName),
		NetworkName: h.cfg.NetworkName,
		RAMMB:       ramMB,
		JVMFlags:    jvmFlagsStr,
		JarVersion:  jarVersion.String,
	}

	ctx := context.Background()
	if err := h.docker.StartProxyContainer(ctx, sanitizedName, cfg); err != nil {
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	ctx := context.Background()
	timeout := 30
	if err := h.docker.StopProxyContainer(ctx, sanitizedName, &timeout); err != nil {
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

	var sanitizedName string
	var hostPort, ramMB int
	var udpPort sql.NullInt64
	var jvmFlags sql.NullString
	var jarVersion sql.NullString
	err = h.db.QueryRow("SELECT sanitized_name, host_port, COALESCE(ram_mb, 512), udp_port, jvm_flags, jar_version FROM proxies WHERE id = ?", id).Scan(&sanitizedName, &hostPort, &ramMB, &udpPort, &jvmFlags, &jarVersion)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	ctx := context.Background()
	timeout := 30

	if err := h.docker.StopProxyContainer(ctx, sanitizedName, &timeout); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to stop proxy: %v", err)})
		return
	}

	h.db.Exec("UPDATE proxies SET status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	containerID, err := h.docker.GetProxyContainerID(ctx, sanitizedName)
	if err == nil && containerID != "" {
		if err := h.docker.WaitForContainer(ctx, containerID, 45*time.Second); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to wait for container to stop: %v", err)})
			return
		}
	}

	jvmFlagsStr := ""
	if jvmFlags.Valid {
		jvmFlagsStr = jvmFlags.String
	}

	cfg := &docker.ProxyContainerConfig{
		Name:        sanitizedName,
		HostPort:    hostPort,
		ProxyPath:   filepath.Join(h.cfg.HostServersDir, sanitizedName),
		NetworkName: h.cfg.NetworkName,
		RAMMB:       ramMB,
		JVMFlags:    jvmFlagsStr,
		UDPPort:     getUDPPort(udpPort),
		JarVersion:  jarVersion.String,
	}

	// AutoRemove is async, so the old container may still exist here with stale
	// port config — force it away so StartProxyContainer recreates with fresh config
	_ = h.docker.RemoveContainer(ctx, "demimine-proxy-"+sanitizedName)

	if err := h.docker.StartProxyContainer(ctx, sanitizedName, cfg); err != nil {
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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
	logs, err := h.docker.GetProxyContainerLogs(ctx, sanitizedName, lines)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get logs"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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
	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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
	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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
	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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
	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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
	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
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

	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

func (h *ProxyHandler) getIconPath(id int64, sanitizedName string) *string {
	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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
	var sanitizedName string
	var motdLine1 sql.NullString
	var motdLine2 sql.NullString
	err = h.db.QueryRow("SELECT name, sanitized_name, motd_line1, motd_line2 FROM proxies WHERE id = ?", id).Scan(&name, &sanitizedName, &motdLine1, &motdLine2)
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

	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
	iconPath := filepath.Join(proxyPath, "icon.png")

	if err := os.WriteFile(iconPath, content, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save icon"})
		return
	}

	if motdLine1.Valid || motdLine2.Valid {
		if err := h.minimotdMgr.CopyMainConfigIcon(sanitizedName, content); err != nil {
			fmt.Printf("Failed to copy icon to MiniMOTD for proxy %d: %v\n", id, err)
		}
		if err := h.minimotdMgr.CreateMainConfig(sanitizedName, motdLine1.String, motdLine2.String, true); err != nil {
			fmt.Printf("Failed to update MiniMOTD main config for proxy %d: %v\n", id, err)
		}
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

	var sanitizedName string
	err = h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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
	var sanitizedName string
	var motdLine1 sql.NullString
	var motdLine2 sql.NullString
	err = h.db.QueryRow("SELECT name, sanitized_name, motd_line1, motd_line2 FROM proxies WHERE id = ?", id).Scan(&name, &sanitizedName, &motdLine1, &motdLine2)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
		return
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, sanitizedName)
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

	if motdLine1.Valid || motdLine2.Valid {
		minimotdIconPath := h.minimotdMgr.GetMainConfigIconPath(sanitizedName)
		os.Remove(minimotdIconPath)
		if err := h.minimotdMgr.CreateMainConfig(sanitizedName, motdLine1.String, motdLine2.String, false); err != nil {
			fmt.Printf("Failed to update MiniMOTD main config for proxy %d: %v\n", id, err)
		}
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

func (h *ProxyHandler) ListAll() ([]models.Proxy, error) {
	rows, err := h.db.Query(`
		SELECT id, name, host_port, ram_mb, forwarding_secret, status, canvas_x, canvas_y, start_on_boot, created_at
		FROM proxies`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var proxies []models.Proxy
	for rows.Next() {
		var p models.Proxy
		var forwardingSecret sql.NullString
		if err := rows.Scan(&p.ID, &p.Name, &p.HostPort, &p.RAMMB, &forwardingSecret, &p.Status, &p.CanvasX, &p.CanvasY, &p.StartOnBoot, &p.CreatedAt); err != nil {
			return nil, err
		}
		if forwardingSecret.Valid {
			p.ForwardingSecret = forwardingSecret.String
		}
		proxies = append(proxies, p)
	}
	return proxies, nil
}

func (h *ProxyHandler) StopByID(id int64) error {
	var sanitizedName string
	err := h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", id).Scan(&sanitizedName)
	if err != nil {
		return err
	}

	ctx := context.Background()
	timeout := 30
	if err := h.docker.StopProxyContainer(ctx, sanitizedName, &timeout); err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			return err
		}
	}

	h.db.Exec("UPDATE proxies SET status = 'stopped' WHERE id = ?", id)
	return nil
}

func (h *ProxyHandler) StartByID(id int64) error {
	var sanitizedName string
	var hostPort, ramMB int
	var jvmFlags sql.NullString
	var udpPort sql.NullInt64
	var jarVersion sql.NullString
	err := h.db.QueryRow(`
		SELECT sanitized_name, host_port, COALESCE(ram_mb, 512), jvm_flags, udp_port, jar_version FROM proxies WHERE id = ?`, id).
		Scan(&sanitizedName, &hostPort, &ramMB, &jvmFlags, &udpPort, &jarVersion)
	if err != nil {
		return err
	}

	jvmFlagsStr := ""
	if jvmFlags.Valid {
		jvmFlagsStr = jvmFlags.String
	}

	udpPortVal := 0
	if udpPort.Valid {
		udpPortVal = int(udpPort.Int64)
	}

	cfg := docker.ProxyContainerConfig{
		Name:        sanitizedName,
		HostPort:    hostPort,
		UDPPort:     udpPortVal,
		ProxyPath:   filepath.Join(h.cfg.HostServersDir, sanitizedName),
		NetworkName: h.cfg.NetworkName,
		RAMMB:       ramMB,
		JVMFlags:    jvmFlagsStr,
		JarVersion:  jarVersion.String,
	}

	ctx := context.Background()
	if err := h.docker.StartProxyContainer(ctx, sanitizedName, &cfg); err != nil {
		return err
	}

	h.db.Exec("UPDATE proxies SET status = 'running' WHERE id = ?", id)
	return nil
}
