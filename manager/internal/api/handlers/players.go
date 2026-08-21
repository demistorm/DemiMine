package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/go-chi/chi/v5"
)

type PlayerHandler struct {
	db     *sql.DB
	docker *docker.Client
	cfg    *config.Config
}

func NewPlayerHandler(db *sql.DB, dockerClient *docker.Client, cfg *config.Config) *PlayerHandler {
	return &PlayerHandler{db: db, docker: dockerClient, cfg: cfg}
}

type AutoShutdownServer struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	RAMMB int    `json:"ram_mb"`
}

type ServerStatusResponse struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	PlayerCount int    `json:"player_count"`
}

type PlayerJoinRequest struct {
	UUID       string `json:"uuid" binding:"required"`
	Name       string `json:"name" binding:"required"`
	ServerName string `json:"server_name" binding:"required"`
}

type PlayerLeaveRequest struct {
	UUID       string `json:"uuid" binding:"required"`
	ServerName string `json:"server_name"`
}

type PlayerInfo struct {
	UUID     string    `json:"uuid"`
	Name     string    `json:"name"`
	JoinedAt time.Time `json:"joined_at"`
}

func (h *PlayerHandler) Join(w http.ResponseWriter, r *http.Request) {
	var req PlayerJoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	var serverID int64
	err := h.db.QueryRow("SELECT id FROM servers WHERE sanitized_name = ?", req.ServerName).Scan(&serverID)
	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}

	_, err = h.db.Exec(`
		INSERT INTO players (name, uuid, server_id, joined_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(uuid) DO UPDATE SET
			name = excluded.name,
			server_id = excluded.server_id,
			joined_at = excluded.joined_at
	`, req.Name, req.UUID, serverID, time.Now())

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to record player join: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *PlayerHandler) Leave(w http.ResponseWriter, r *http.Request) {
	var req PlayerLeaveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	// server_name optional: without it we clear the player wherever they are,
	// which the proxy uses when a player drops before fully connecting
	var result sql.Result
	var err error
	if req.ServerName == "" {
		result, err = h.db.Exec("DELETE FROM players WHERE uuid = ?", req.UUID)
	} else {
		var serverID int64
		err = h.db.QueryRow("SELECT id FROM servers WHERE sanitized_name = ?", req.ServerName).Scan(&serverID)
		if err != nil {
			if err == sql.ErrNoRows {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusNotFound)
				json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
			return
		}
		result, err = h.db.Exec("DELETE FROM players WHERE uuid = ? AND server_id = ?", req.UUID, serverID)
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to record player leave"})
		return
	}

	if rows, _ := result.RowsAffected(); rows == 0 {
		// nothing to remove, player wasn't tracked anyway
		log.Printf("Player leave for %s (server %q) matched no rows", req.UUID, req.ServerName)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *PlayerHandler) GetServerPlayers(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	if serverID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server id required"})
		return
	}

	rows, err := h.db.Query(`
		SELECT uuid, name, joined_at
		FROM players
		WHERE server_id = ?
		ORDER BY joined_at ASC
	`, serverID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to query players"})
		return
	}
	defer rows.Close()

	players := []PlayerInfo{}
	for rows.Next() {
		var p PlayerInfo
		if err := rows.Scan(&p.UUID, &p.Name, &p.JoinedAt); err != nil {
			continue
		}
		players = append(players, p)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"server_id": serverID,
		"players":   players,
	})
}

func (h *PlayerHandler) GetAutoShutdownServers(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, sanitized_name, ram_mb
		FROM servers
		WHERE status = 'running'
		ORDER BY id ASC
	`)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to query servers"})
		return
	}
	defer rows.Close()

	servers := []AutoShutdownServer{}
	for rows.Next() {
		var s AutoShutdownServer
		if err := rows.Scan(&s.ID, &s.Name, &s.RAMMB); err != nil {
			continue
		}
		servers = append(servers, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(servers)
}

func (h *PlayerHandler) GetServerStatus(w http.ResponseWriter, r *http.Request) {
	serverName := chi.URLParam(r, "name")

	if serverName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server name required"})
		return
	}

	var status string
	var hostPort sql.NullInt64
	err := h.db.QueryRow(`
		SELECT status, host_port
		FROM servers
		WHERE sanitized_name = ?
	`, serverName).Scan(&status, &hostPort)

	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}

	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}

	var playerCount int
	_ = h.db.QueryRow(`
		SELECT COUNT(*)
		FROM players
		WHERE server_id = (SELECT id FROM servers WHERE sanitized_name = ?)
	`, serverName).Scan(&playerCount)

	if status == "running" && hostPort.Valid {
		containerName := "demimine-" + serverName
		address := net.JoinHostPort(containerName, strconv.Itoa(int(hostPort.Int64)))
		conn, err := net.DialTimeout("tcp", address, 2*time.Second)
		if err != nil {
			status = "starting"
		} else {
			conn.Close()
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ServerStatusResponse{
		Name:        serverName,
		Status:      status,
		PlayerCount: playerCount,
	})
}

func (h *PlayerHandler) StopServerByName(w http.ResponseWriter, r *http.Request) {
	serverName := chi.URLParam(r, "name")

	if serverName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server name required"})
		return
	}

	var serverID string
	var sanitizedName string
	err := h.db.QueryRow("SELECT id, sanitized_name FROM servers WHERE sanitized_name = ?", serverName).Scan(&serverID, &sanitizedName)
	if err != nil {
		if err == sql.ErrNoRows {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}

	ctx := context.Background()
	timeout := 30
	if err := h.docker.StopContainer(ctx, sanitizedName, &timeout); err != nil {
		if !strings.Contains(err.Error(), "No such container") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to stop server: %v", err)})
			return
		}
	}

	h.db.Exec("UPDATE servers SET status = 'stopped', updated_at = CURRENT_TIMESTAMP WHERE id = ?", serverID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *PlayerHandler) StartServerByName(w http.ResponseWriter, r *http.Request) {
	serverName := chi.URLParam(r, "name")

	if serverName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server name required"})
		return
	}

	var id int64
	var name, sanitizedName, serverType, version string
	var ramMB int
	var hostPort sql.NullInt64
	var udpPort sql.NullInt64
	var jvmFlags sql.NullString
	var javaOverride sql.NullString
	err := h.db.QueryRow(`
		SELECT id, name, sanitized_name, type, version, ram_mb, host_port, udp_port, jvm_flags, java_override FROM servers WHERE sanitized_name = ?`, serverName).
		Scan(&id, &name, &sanitizedName, &serverType, &version, &ramMB, &hostPort, &udpPort, &jvmFlags, &javaOverride)
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
		return
	}
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "database error"})
		return
	}

	port := 0
	if hostPort.Valid {
		port = int(hostPort.Int64)
	}

	jvmFlagsStr := ""
	if jvmFlags.Valid {
		jvmFlagsStr = jvmFlags.String
	}

	javaOverrideStr := ""
	if javaOverride.Valid {
		javaOverrideStr = javaOverride.String
	}

	cfg := &docker.ServerContainerConfig{
		Name:         sanitizedName,
		ServerType:   serverType,
		Version:      version,
		RAMMB:        ramMB,
		JVMFlags:     jvmFlagsStr,
		JavaOverride: javaOverrideStr,
		ServerPath:   filepath.Join(h.cfg.HostServersDir, sanitizedName),
		NetworkName:  h.cfg.NetworkName,
		HostPort:     port,
		UDPPort:      getUDPPort(udpPort),
	}

	ctx := context.Background()
	if err := h.docker.StartContainer(ctx, sanitizedName, cfg); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to start server: %v", err)})
		return
	}

	h.db.Exec("UPDATE servers SET status = 'running', updated_at = CURRENT_TIMESTAMP WHERE id = ?", id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
