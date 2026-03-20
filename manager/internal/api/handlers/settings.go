package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type SettingsHandler struct {
	db *sql.DB
}

func NewSettingsHandler(db *sql.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

func (h *SettingsHandler) Get(w http.ResponseWriter, r *http.Request) {
	settings := make(map[string]string)

	rows, err := h.db.Query("SELECT key, value FROM settings")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to get settings"})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err == nil {
			settings[key] = value
		}
	}

	if _, ok := settings["proxy_mc_version"]; !ok {
		settings["proxy_mc_version"] = "1.21.11"
	}
	if _, ok := settings["backup_time"]; !ok {
		settings["backup_time"] = "03:00"
	}
	if _, ok := settings["backup_interval_days"]; !ok {
		settings["backup_interval_days"] = "3"
	}
	if _, ok := settings["retention_count"]; !ok {
		settings["retention_count"] = "2"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

type UpdateSettingsRequest struct {
	ProxyMCVersion     string `json:"proxy_mc_version"`
	BackupTime         string `json:"backup_time"`
	BackupIntervalDays string `json:"backup_interval_days"`
	RetentionCount     int    `json:"retention_count"`
}

func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	var req UpdateSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	if req.ProxyMCVersion != "" {
		h.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('proxy_mc_version', ?)", req.ProxyMCVersion)
	}
	if req.BackupTime != "" {
		h.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('backup_time', ?)", req.BackupTime)
	}
	if req.BackupIntervalDays != "" {
		h.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('backup_interval_days', ?)", req.BackupIntervalDays)
	}
	if req.RetentionCount > 0 {
		h.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('retention_count', ?)", req.RetentionCount)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
