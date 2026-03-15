package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/mc"

	"github.com/go-chi/chi/v5"
)

type JarUpdateHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewJarUpdateHandler(db *sql.DB, cfg *config.Config) *JarUpdateHandler {
	return &JarUpdateHandler{db: db, cfg: cfg}
}

func (h *JarUpdateHandler) CheckServerJarUpdate(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	var serverType, serverVersion string
	var jarBuild sql.NullInt64

	err := h.db.QueryRow(
		"SELECT type, version, jar_build FROM servers WHERE id = ?",
		serverID,
	).Scan(&serverType, &serverVersion, &jarBuild)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	serverType = strings.ToLower(serverType)

	build := 0
	if jarBuild.Valid {
		build = int(jarBuild.Int64)
	}

	var updateInfo *mc.JarUpdateInfo
	var checkErr error

	switch serverType {
	case "paper":
		updateInfo, checkErr = mc.CheckPaperUpdate(serverVersion, build)
	case "purpur":
		var currentHash string
		hashErr := h.db.QueryRow(
			"SELECT jar_hash FROM servers WHERE id = ?",
			serverID,
		).Scan(&currentHash)
		if hashErr != nil && hashErr != sql.ErrNoRows {
			currentHash = ""
		}
		updateInfo, checkErr = mc.CheckPurpurUpdate(serverVersion, currentHash)
	default:
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"has_update": false,
			"message":    "JAR updates not supported for this server type",
		})
		return
	}

	if checkErr != nil {
		http.Error(w, fmt.Sprintf("Failed to check for updates: %v", checkErr), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updateInfo)
}

func (h *JarUpdateHandler) UpdateServerJar(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	var serverType, serverVersion, serverName, status string

	err := h.db.QueryRow(
		"SELECT type, version, name, status FROM servers WHERE id = ?",
		serverID,
	).Scan(&serverType, &serverVersion, &serverName, &status)
	if err != nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	if status == "running" {
		http.Error(w, "Server must be stopped before updating", http.StatusConflict)
		return
	}

	serverPath := filepath.Join(h.cfg.ServersDir, serverName)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create server directory: %v", err), http.StatusInternalServerError)
		return
	}
	jarPath := filepath.Join(serverPath, "server.jar")

	var build int
	var hash string

	switch serverType {
	case "paper":
		build, hash, err = mc.DownloadPaperJar(serverVersion, jarPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to download JAR: %v", err), http.StatusInternalServerError)
			return
		}
	case "purpur":
		hash, err = mc.DownloadPurpurJar(serverVersion, jarPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to download JAR: %v", err), http.StatusInternalServerError)
			return
		}
	default:
		http.Error(w, "JAR updates not supported for this server type", http.StatusBadRequest)
		return
	}

	switch serverType {
	case "paper":
		_, err = h.db.Exec(
			"UPDATE servers SET jar_build = ?, jar_hash = ? WHERE id = ?",
			build, hash, serverID,
		)
		if err == nil {
			mc.ClearPaperCache(serverVersion, build)
		}
	case "purpur":
		_, err = h.db.Exec(
			"UPDATE servers SET jar_hash = ? WHERE id = ?",
			hash, serverID,
		)
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update database: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "JAR updated successfully",
	})
}

func (h *JarUpdateHandler) CheckProxyJarUpdate(w http.ResponseWriter, r *http.Request) {
	proxyID := chi.URLParam(r, "id")

	var jarVersion sql.NullString
	var jarBuild sql.NullInt64

	err := h.db.QueryRow(
		"SELECT jar_version, jar_build FROM proxies WHERE id = ?",
		proxyID,
	).Scan(&jarVersion, &jarBuild)
	if err != nil {
		if err == sql.ErrNoRows {
			jarVersion.String = ""
			jarVersion.Valid = true
			jarBuild.Int64 = 0
			jarBuild.Valid = true
		} else {
			http.Error(w, "Proxy not found", http.StatusNotFound)
			return
		}
	}

	version := ""
	if jarVersion.Valid {
		version = jarVersion.String
	}
	build := 0
	if jarBuild.Valid {
		build = int(jarBuild.Int64)
	}

	updateInfo, err := mc.CheckVelocityUpdate(version, build)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to check for updates: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updateInfo)
}

func (h *JarUpdateHandler) UpdateProxyJar(w http.ResponseWriter, r *http.Request) {
	proxyID := chi.URLParam(r, "id")

	var status, proxyName string

	err := h.db.QueryRow(
		"SELECT status, name FROM proxies WHERE id = ?",
		proxyID,
	).Scan(&status, &proxyName)
	if err != nil {
		http.Error(w, "Proxy not found", http.StatusNotFound)
		return
	}

	if status == "running" {
		http.Error(w, "Proxy must be stopped before updating", http.StatusConflict)
		return
	}

	proxyPath := filepath.Join(h.cfg.ServersDir, proxyName)
	if err := os.MkdirAll(proxyPath, 0755); err != nil {
		http.Error(w, fmt.Sprintf("Failed to create proxy directory: %v", err), http.StatusInternalServerError)
		return
	}
	jarPath := filepath.Join(proxyPath, "velocity.jar")

	version, build, err := mc.DownloadVelocityJar(jarPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to download JAR: %v", err), http.StatusInternalServerError)
		return
	}

	_, err = h.db.Exec(
		"UPDATE proxies SET jar_version = ?, jar_build = ? WHERE id = ?",
		version, build, proxyID,
	)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update database: %v", err), http.StatusInternalServerError)
		return
	}

	mc.ClearVelocityCache(version, build)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "success",
		"message": "JAR updated successfully",
	})
}
