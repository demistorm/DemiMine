package handlers

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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

	if _, ok := settings["server_timezone"]; !ok {
		tz := os.Getenv("TZ")
		if tz == "" {
			tz = "UTC"
		}
		settings["server_timezone"] = tz
	}

	if _, ok := settings["background_texture_scale"]; !ok {
		settings["background_texture_scale"] = "4"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(settings)
}

type UpdateSettingsRequest struct {
	ProxyMCVersion         string `json:"proxy_mc_version"`
	BackupTime             string `json:"backup_time"`
	BackupIntervalDays     string `json:"backup_interval_days"`
	RetentionCount         int    `json:"retention_count"`
	BackgroundTextureScale int    `json:"background_texture_scale"`
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
	if req.BackgroundTextureScale > 0 {
		h.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('background_texture_scale', ?)", req.BackgroundTextureScale)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

const (
	dataDir            = "/data"
	backgroundTexture  = "background_texture.png"
	serverTileTexture  = "server_tile_texture.png"
	proxyTileTexture   = "proxy_tile_texture.png"
	maxTextureFileSize = 1 << 20 // 1MB
)

func (h *SettingsHandler) UploadBackgroundTexture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTextureFileSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large"})
		return
	}

	file, handler, err := r.FormFile("texture")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	contentType := handler.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/png") && !strings.HasPrefix(contentType, "image/x-png") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "only PNG files are allowed"})
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read file"})
		return
	}

	if len(data) > maxTextureFileSize {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large"})
		return
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create data directory"})
		return
	}

	texturePath := filepath.Join(dataDir, backgroundTexture)
	if err := os.WriteFile(texturePath, data, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save texture"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *SettingsHandler) ServeBackgroundTexture(w http.ResponseWriter, r *http.Request) {
	texturePath := filepath.Join(dataDir, backgroundTexture)

	data, err := os.ReadFile(texturePath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "background texture not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read texture"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *SettingsHandler) DeleteBackgroundTexture(w http.ResponseWriter, r *http.Request) {
	texturePath := filepath.Join(dataDir, backgroundTexture)

	if err := os.Remove(texturePath); err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "background texture not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete texture"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *SettingsHandler) UploadServerTileTexture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTextureFileSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large"})
		return
	}

	file, handler, err := r.FormFile("texture")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	contentType := handler.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/png") && !strings.HasPrefix(contentType, "image/x-png") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "only PNG files are allowed"})
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read file"})
		return
	}

	if len(data) > maxTextureFileSize {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large"})
		return
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create data directory"})
		return
	}

	texturePath := filepath.Join(dataDir, serverTileTexture)
	if err := os.WriteFile(texturePath, data, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save texture"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *SettingsHandler) ServeServerTileTexture(w http.ResponseWriter, r *http.Request) {
	texturePath := filepath.Join(dataDir, serverTileTexture)

	data, err := os.ReadFile(texturePath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server tile texture not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read texture"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *SettingsHandler) DeleteServerTileTexture(w http.ResponseWriter, r *http.Request) {
	texturePath := filepath.Join(dataDir, serverTileTexture)

	if err := os.Remove(texturePath); err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server tile texture not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete texture"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *SettingsHandler) UploadProxyTileTexture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxTextureFileSize)
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large"})
		return
	}

	file, handler, err := r.FormFile("texture")
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "no file uploaded"})
		return
	}
	defer file.Close()

	contentType := handler.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/png") && !strings.HasPrefix(contentType, "image/x-png") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "only PNG files are allowed"})
		return
	}

	data, err := io.ReadAll(file)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read file"})
		return
	}

	if len(data) > maxTextureFileSize {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "file too large"})
		return
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create data directory"})
		return
	}

	texturePath := filepath.Join(dataDir, proxyTileTexture)
	if err := os.WriteFile(texturePath, data, 0644); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to save texture"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *SettingsHandler) ServeProxyTileTexture(w http.ResponseWriter, r *http.Request) {
	texturePath := filepath.Join(dataDir, proxyTileTexture)

	data, err := os.ReadFile(texturePath)
	if err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "proxy tile texture not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to read texture"})
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

func (h *SettingsHandler) DeleteProxyTileTexture(w http.ResponseWriter, r *http.Request) {
	texturePath := filepath.Join(dataDir, proxyTileTexture)

	if err := os.Remove(texturePath); err != nil {
		if os.IsNotExist(err) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "proxy tile texture not found"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete texture"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
