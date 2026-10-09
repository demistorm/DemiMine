package handlers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/demimine/manager/internal/api/middleware"

	"github.com/go-chi/chi/v5"
)

// manualJarFilename returns the name of the jar the entrypoint actually runs (or
// installs from) for each server type. Manual updates rename the uploaded jar to this.
func manualJarFilename(serverType string) (string, bool) {
	switch serverType {
	case "paper", "purpur", "nanolimbo":
		return "server.jar", true
	case "fabric":
		// standalone Fabric Server Launcher (fabricmc.net/use/server)
		return "fabric-server-launch.jar", true
	case "neoforge":
		return "neoforge-installer.jar", true
	case "forge":
		return "forge-installer.jar", true
	default:
		return "", false
	}
}

// manualUpdateCleanupPaths lists derived install state to drop after a manual jar
// swap so the entrypoint regenerates it on next start.
func manualUpdateCleanupPaths(serverType string) []string {
	switch serverType {
	case "fabric":
		// the standalone launcher keeps its own copies under .fabric/server and
		// ignores the stub-flow files; stale .fabric causes classloading errors
		return []string{"fabric-server-launcher.properties", ".fabric", "server.jar"}
	case "neoforge", "forge":
		// entrypoint re-runs the installer when run.sh is missing, which updates
		// libraries in place (per NeoForge/Forge docs)
		return []string{"run.sh"}
	default:
		return nil
	}
}

func (h *JarUpdateHandler) ManualServerJarUpdate(w http.ResponseWriter, r *http.Request) {
	serverID := chi.URLParam(r, "id")

	var serverType, serverName, status string
	err := h.db.QueryRow(
		"SELECT type, sanitized_name, status FROM servers WHERE id = ?",
		serverID,
	).Scan(&serverType, &serverName, &status)
	if err != nil {
		middleware.WriteJSONError(w, http.StatusNotFound, "server not found")
		return
	}

	if status == "running" {
		middleware.WriteJSONError(w, http.StatusConflict, "server must be stopped before updating")
		return
	}

	serverType = strings.ToLower(serverType)

	jarName, ok := manualJarFilename(serverType)
	if !ok {
		middleware.WriteJSONError(w, http.StatusBadRequest, "manual JAR update not supported for this server type")
		return
	}

	if err := r.ParseMultipartForm(1 << 30); err != nil {
		middleware.WriteJSONError(w, http.StatusBadRequest, "failed to parse multipart form")
		return
	}

	version := strings.TrimSpace(r.FormValue("version"))
	if version == "" || strings.ContainsAny(version, `/\ `) || len(version) > 64 {
		middleware.WriteJSONError(w, http.StatusBadRequest, "invalid version")
		return
	}

	build := 0
	buildStr := strings.TrimSpace(r.FormValue("build"))
	usesBuilds := serverType == "paper" || serverType == "purpur"
	if buildStr != "" && usesBuilds {
		build, err = strconv.Atoi(buildStr)
		if err != nil || build < 0 {
			middleware.WriteJSONError(w, http.StatusBadRequest, "invalid build number")
			return
		}
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		middleware.WriteJSONError(w, http.StatusBadRequest, "missing file")
		return
	}
	defer file.Close()

	// cheap jar sanity check: zip magic
	magic := make([]byte, 2)
	if _, err := io.ReadFull(file, magic); err != nil || string(magic) != "PK" {
		middleware.WriteJSONError(w, http.StatusBadRequest, "file is not a valid JAR")
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to read file")
		return
	}

	serverPath := filepath.Join(h.cfg.ServersDir, serverName)
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to create server directory")
		return
	}

	tmpPath := filepath.Join(serverPath, ".upload_temp.jar")
	written, err := func() (int64, error) {
		dst, err := os.Create(tmpPath)
		if err != nil {
			return 0, err
		}
		defer dst.Close()
		return io.Copy(dst, file)
	}()
	if err != nil || written == 0 {
		os.Remove(tmpPath)
		if err == nil {
			middleware.WriteJSONError(w, http.StatusBadRequest, "uploaded file is empty")
			return
		}
		middleware.WriteJSONInternalError(w, err, "failed to save uploaded file")
		return
	}

	jarPath := filepath.Join(serverPath, jarName)
	if err := os.Rename(tmpPath, jarPath); err != nil {
		os.Remove(tmpPath)
		middleware.WriteJSONInternalError(w, err, "failed to place JAR")
		return
	}

	for _, p := range manualUpdateCleanupPaths(serverType) {
		os.RemoveAll(filepath.Join(serverPath, p))
	}

	if usesBuilds {
		_, err = h.db.Exec(
			"UPDATE servers SET version = ?, jar_build = ?, jar_hash = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			version, build, serverID,
		)
	} else {
		_, err = h.db.Exec(
			"UPDATE servers SET version = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?",
			version, serverID,
		)
	}
	if err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to update database")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "success",
		"message":   "JAR updated successfully",
		"version":   version,
		"jar_build": build,
	})
}
