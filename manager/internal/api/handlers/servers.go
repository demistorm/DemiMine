package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/demimine/manager/internal/models"
	"github.com/go-chi/chi/v5"
)

type ServerHandler struct {
	db *sql.DB
}

func NewServerHandler(db *sql.DB) *ServerHandler {
	return &ServerHandler{db: db}
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
		       s.host_port, s.status, s.created_at,
		       COALESCE((SELECT COUNT(*) FROM players WHERE server_id = s.id), 0) as player_count
		FROM servers s
		LEFT JOIN proxies p ON s.proxy_id = p.id
		ORDER BY s.created_at DESC
	`)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to list servers",
		})
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
		
		err := rows.Scan(
			&s.ID, &s.Name, &s.Type, &s.Version, &proxyID, &proxyName, &s.RAMMB, &domain,
			&s.BackupIntervalDays, &s.AutoShutdownMinutes, &scheduledStart, &scheduledStop,
			&hostPort, &s.Status, &s.CreatedAt, &s.PlayerCount,
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
}

func (h *ServerHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req CreateServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid request body",
		})
		return
	}
	
	if req.Name == "" || req.Type == "" || req.Version == "" || req.RAMMB == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "name, type, version, and ram_mb are required",
		})
		return
	}
	
	if req.ProxyID == nil && req.HostPort == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "either proxy_id or host_port must be specified",
		})
		return
	}
	
	force := r.URL.Query().Get("force") == "true"
	
	if req.HostPort != nil && !force {
		var existingName string
		err := h.db.QueryRow(
			"SELECT name FROM servers WHERE host_port = ?",
			*req.HostPort,
		).Scan(&existingName)
		
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":    "port_in_use",
				"port":     *req.HostPort,
				"used_by": existingName,
			})
			return
		}
		
		var proxyName string
		err = h.db.QueryRow(
			"SELECT name FROM proxies WHERE host_port = ?",
			*req.HostPort,
		).Scan(&proxyName)
		
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":    "port_in_use",
				"port":     *req.HostPort,
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
		json.NewEncoder(w).Encode(map[string]string{
			"error": "server name already exists",
		})
		return
	}
	
	if req.ProxyID != nil {
		var proxyExists bool
		err = h.db.QueryRow("SELECT 1 FROM proxies WHERE id = ?", *req.ProxyID).Scan(&proxyExists)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "proxy not found",
			})
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
	
	result, err := h.db.Exec(`
		INSERT INTO servers (name, type, version, proxy_id, host_port, ram_mb, domain, backup_interval_days, auto_shutdown_minutes, scheduled_start, scheduled_stop, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'stopped')
	`, req.Name, req.Type, req.Version, req.ProxyID, req.HostPort, req.RAMMB, req.Domain, req.BackupIntervalDays, req.AutoShutdownMinutes, req.ScheduledStart, req.ScheduledStop)
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to create server",
		})
		return
	}
	
	id, _ := result.LastInsertId()
	
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]int64{
		"id": id,
	})
}

func (h *ServerHandler) Get(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid server id",
		})
		return
	}
	
	var s models.ServerResponse
	var proxyID sql.NullInt64
	var proxyName sql.NullString
	var domain sql.NullString
	var scheduledStart sql.NullString
	var scheduledStop sql.NullString
	var hostPort sql.NullInt64
	
	err = h.db.QueryRow(`
		SELECT s.id, s.name, s.type, s.version, s.proxy_id, p.name, s.ram_mb, s.domain,
		       s.backup_interval_days, s.auto_shutdown_minutes, s.scheduled_start, s.scheduled_stop,
		       s.host_port, s.status, s.created_at,
		       COALESCE((SELECT COUNT(*) FROM players WHERE server_id = s.id), 0) as player_count
		FROM servers s
		LEFT JOIN proxies p ON s.proxy_id = p.id
		WHERE s.id = ?
	`, id).Scan(
		&s.ID, &s.Name, &s.Type, &s.Version, &proxyID, &proxyName, &s.RAMMB, &domain,
		&s.BackupIntervalDays, &s.AutoShutdownMinutes, &scheduledStart, &scheduledStop,
		&hostPort, &s.Status, &s.CreatedAt, &s.PlayerCount,
	)
	
	if err == sql.ErrNoRows {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "server not found",
		})
		return
	}
	
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to get server",
		})
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
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s)
}

func (h *ServerHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid server id",
		})
		return
	}
	
	result, err := h.db.Exec("DELETE FROM servers WHERE id = ?", id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "failed to delete server",
		})
		return
	}
	
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "server not found",
		})
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{
		"success": true,
	})
}
