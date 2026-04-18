package spark

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type Handler struct {
	service *Service
	db      *sql.DB
}

func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

func (h *Handler) SetDB(db *sql.DB) {
	h.db = db
}

func (h *Handler) getServerContainerName(serverID string) (string, error) {
	if h.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	var sanitizedName string
	err := h.db.QueryRow("SELECT sanitized_name FROM servers WHERE id = ?", serverID).Scan(&sanitizedName)
	if err != nil {
		return "", fmt.Errorf("server not found: %w", err)
	}

	return "demimine-" + sanitizedName, nil
}

func (h *Handler) getProxyContainerName(proxyID string) (string, error) {
	if h.db == nil {
		return "", fmt.Errorf("database not initialized")
	}

	var sanitizedName string
	err := h.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", proxyID).Scan(&sanitizedName)
	if err != nil {
		return "", fmt.Errorf("proxy not found: %w", err)
	}

	return "demimine-proxy-" + sanitizedName, nil
}

type ProfileStartResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ProfileStopResponse struct {
	Success bool   `json:"success"`
	URL     string `json:"url,omitempty"`
	Message string `json:"message"`
}

func (h *Handler) StartServerProfiler(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	if serverID == "" {
		h.sendError(w, "server ID required", http.StatusBadRequest)
		return
	}

	containerName, err := h.getServerContainerName(serverID)
	if err != nil {
		h.sendError(w, "failed to get server", http.StatusNotFound)
		return
	}

	err = h.service.StartProfiler(containerName)
	if err != nil {
		h.sendError(w, "failed to start profiler: "+err.Error(), http.StatusInternalServerError)
		log.Printf("[Spark API] StartServerProfiler failed for %s: %v", containerName, err)
		return
	}

	h.sendJSON(w, ProfileStartResponse{
		Success: true,
		Message: "Profiler started",
	})
}

func (h *Handler) StopServerProfiler(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	if serverID == "" {
		h.sendError(w, "server ID required", http.StatusBadRequest)
		return
	}

	containerName, err := h.getServerContainerName(serverID)
	if err != nil {
		h.sendError(w, "failed to get server", http.StatusNotFound)
		return
	}

	url, err := h.service.StopProfiler(containerName, serverID)
	if err != nil {
		h.sendError(w, "failed to stop profiler: "+err.Error(), http.StatusInternalServerError)
		log.Printf("[Spark API] StopServerProfiler failed for %s: %v", containerName, err)
		return
	}

	if url == "" {
		h.sendJSON(w, ProfileStopResponse{
			Success: true,
			Message: "Profiler stopped (no URL generated)",
		})
		return
	}

	h.sendJSON(w, ProfileStopResponse{
		Success: true,
		URL:     url,
		Message: "Profiler stopped",
	})
}

func (h *Handler) StartProxyProfiler(w http.ResponseWriter, r *http.Request) {
	proxyID := r.PathValue("id")
	if proxyID == "" {
		h.sendError(w, "proxy ID required", http.StatusBadRequest)
		return
	}

	containerName, err := h.getProxyContainerName(proxyID)
	if err != nil {
		h.sendError(w, "failed to get proxy", http.StatusNotFound)
		return
	}

	err = h.service.StartProfiler(containerName)
	if err != nil {
		h.sendError(w, "failed to start profiler: "+err.Error(), http.StatusInternalServerError)
		log.Printf("[Spark API] StartProxyProfiler failed for %s: %v", containerName, err)
		return
	}

	h.sendJSON(w, ProfileStartResponse{
		Success: true,
		Message: "Profiler started",
	})
}

func (h *Handler) StopProxyProfiler(w http.ResponseWriter, r *http.Request) {
	proxyID := r.PathValue("id")
	if proxyID == "" {
		h.sendError(w, "proxy ID required", http.StatusBadRequest)
		return
	}

	containerName, err := h.getProxyContainerName(proxyID)
	if err != nil {
		h.sendError(w, "failed to get proxy", http.StatusNotFound)
		return
	}

	url, err := h.service.StopProfiler(containerName, proxyID)
	if err != nil {
		h.sendError(w, "failed to stop profiler: "+err.Error(), http.StatusInternalServerError)
		log.Printf("[Spark API] StopProxyProfiler failed for %s: %v", containerName, err)
		return
	}

	if url == "" {
		h.sendJSON(w, ProfileStopResponse{
			Success: true,
			Message: "Profiler stopped (no URL generated)",
		})
		return
	}

	h.sendJSON(w, ProfileStopResponse{
		Success: true,
		URL:     url,
		Message: "Profiler stopped",
	})
}

func (h *Handler) sendJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func (h *Handler) sendError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": message,
	})
}
