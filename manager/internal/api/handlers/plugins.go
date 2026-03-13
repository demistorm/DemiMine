package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/demimine/manager/internal/modrinth"
	"github.com/demimine/manager/internal/plugin"
	"github.com/go-chi/chi/v5"
)

type PluginHandler struct {
	pluginManager  *plugin.Manager
	db             *sql.DB
	modrinthClient *modrinth.Client
}

func NewPluginHandler(db *sql.DB, serversDir string) *PluginHandler {
	return &PluginHandler{
		pluginManager:  plugin.NewManager(db, serversDir),
		db:             db,
		modrinthClient: modrinth.NewClient(),
	}
}

func (h *PluginHandler) GetInstalled(w http.ResponseWriter, r *http.Request) {
	targetType := chi.URLParam(r, "type")
	targetIDStr := chi.URLParam(r, "id")

	if targetType != "server" && targetType != "proxy" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target type"})
		return
	}

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target id"})
		return
	}

	plugins, err := h.pluginManager.GetInstalled(targetType, targetID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(plugins)
}

type InstallPluginRequest struct {
	ProjectID   string `json:"project_id"`
	VersionID   string `json:"version_id,omitempty"`
	GameVersion string `json:"game_version,omitempty"`
}

func (h *PluginHandler) Install(w http.ResponseWriter, r *http.Request) {
	targetType := chi.URLParam(r, "type")
	targetIDStr := chi.URLParam(r, "id")
	if targetType != "server" && targetType != "proxy" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target type"})
		return
	}

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target id"})
		return
	}

	var req InstallPluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	var loaders []string
	var gameVersion string
	var pluginsDir string

	if targetType == "server" {
		var serverType string
		err := h.db.QueryRow("SELECT type, version, name FROM servers WHERE id = ?", targetID).Scan(&serverType, &gameVersion, &pluginsDir)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		loaders = modrinth.GetLoaderForServerType(serverType)
	} else {
		gameVersion = req.GameVersion
		pluginsDir = "pluginsDir"
		loaders = []string{"velocity"}
		err := h.db.QueryRow("SELECT name FROM proxies WHERE id = ?", targetID).Scan(&pluginsDir)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "proxy not found"})
			return
		}
	}

	if len(loaders) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server type does not support plugins"})
		return
	}

	installed, err := h.pluginManager.Install(plugin.InstallOptions{
		TargetType:  targetType,
		TargetID:    targetID,
		ProjectID:   req.ProjectID,
		VersionID:   req.VersionID,
		GameVersion: gameVersion,
		Loaders:     loaders,
		PluginsDir:  pluginsDir,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if installed != nil {
		deps, _ := h.getDependenciesRecursive(req.ProjectID, gameVersion, loaders, pluginsDir, targetType, targetID)
		if len(deps) > 0 {
			installed.Dependencies = deps
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(installed)
}

func (h *PluginHandler) getDependenciesRecursive(projectID, gameVersion string, loaders []string, pluginsDir string, targetType string, targetID int64) ([]plugin.InstalledPlugin, error) {
	deps, err := h.pluginManager.GetDependencies(projectID, gameVersion, loaders)
	if err != nil {
		return nil, nil
	}

	var installedDeps []plugin.InstalledPlugin
	for _, dep := range deps {
		if dep.DependencyType != "required" {
			continue
		}
		if dep.ProjectID == "" {
			continue
		}
		existing, _ := h.pluginManager.GetInstalledByProject(targetType, targetID, dep.ProjectID)
		if existing != nil {
			continue
		}
		depPlugin, err := h.pluginManager.Install(plugin.InstallOptions{
			TargetType:  targetType,
			TargetID:    targetID,
			ProjectID:   dep.ProjectID,
			GameVersion: gameVersion,
			Loaders:     loaders,
			PluginsDir:  pluginsDir,
		})
		if err != nil {
			continue
		}
		installedDeps = append(installedDeps, *depPlugin)
		transitiveDeps, _ := h.getDependenciesRecursive(dep.ProjectID, gameVersion, loaders, pluginsDir, targetType, targetID)
		if len(transitiveDeps) > 0 {
			installedDeps = append(installedDeps, transitiveDeps...)
		}
	}
	return installedDeps, nil
}

func (h *PluginHandler) Uninstall(w http.ResponseWriter, r *http.Request) {
	targetType := chi.URLParam(r, "type")
	targetIDStr := chi.URLParam(r, "id")
	projectID := chi.URLParam(r, "project_id")
	if targetType != "server" && targetType != "proxy" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target type"})
		return
	}

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target id"})
		return
	}

	if err := h.pluginManager.Uninstall(targetType, targetID, projectID); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func (h *PluginHandler) CheckUpdates(w http.ResponseWriter, r *http.Request) {
	targetType := chi.URLParam(r, "type")
	targetIDStr := chi.URLParam(r, "id")
	if targetType != "server" && targetType != "proxy" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target type"})
		return
	}

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target id"})
		return
	}

	var gameVersion string
	var loaders []string

	if targetType == "server" {
		var serverType string
		err := h.db.QueryRow("SELECT type, version FROM servers WHERE id = ?", targetID).Scan(&serverType, &gameVersion)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		loaders = modrinth.GetLoaderForServerType(serverType)
	} else {
		loaders = []string{"velocity"}
	}

	updates, err := h.pluginManager.CheckForUpdates(targetType, targetID, gameVersion, loaders)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updates)
}

type UpdatePluginRequest struct {
	GameVersion string `json:"game_version,omitempty"`
}

func (h *PluginHandler) Update(w http.ResponseWriter, r *http.Request) {
	targetType := chi.URLParam(r, "type")
	targetIDStr := chi.URLParam(r, "id")
	projectID := chi.URLParam(r, "project_id")
	if targetType != "server" && targetType != "proxy" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target type"})
		return
	}

	targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid target id"})
		return
	}

	var req UpdatePluginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req = UpdatePluginRequest{}
	}

	var loaders []string
	var gameVersion string

	if targetType == "server" {
		var serverType string
		err := h.db.QueryRow("SELECT type, version FROM servers WHERE id = ?", targetID).Scan(&serverType, &gameVersion)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "server not found"})
			return
		}
		loaders = modrinth.GetLoaderForServerType(serverType)
	} else {
		loaders = []string{"velocity"}
	}

	if req.GameVersion != "" {
		gameVersion = req.GameVersion
	}

	updated, err := h.pluginManager.Update(targetType, targetID, projectID, gameVersion, loaders)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}
