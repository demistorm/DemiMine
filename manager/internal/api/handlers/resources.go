package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/go-chi/chi/v5"
)

type ResourceHandler struct {
	db     *sql.DB
	docker *docker.Client
	cfg    *config.Config
}

func NewResourceHandler(db *sql.DB, dockerClient *docker.Client, cfg *config.Config) *ResourceHandler {
	return &ResourceHandler{db: db, docker: dockerClient, cfg: cfg}
}

type ContainerResource struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	UsageMB int64  `json:"usage_mb"`
}

type SystemResources struct {
	UsedMB      int64               `json:"used_mb"`
	MaxMB       int                 `json:"max_mb"`
	AvailableMB int64               `json:"available_mb"`
	Containers  []ContainerResource `json:"containers"`
}

type CanStartResponse struct {
	CanStart         bool   `json:"can_start"`
	CurrentUsageMB   int64  `json:"current_usage_mb"`
	ProjectedUsageMB int64  `json:"projected_usage_mb"`
	MaxMB            int    `json:"max_mb"`
	Reason           string `json:"reason,omitempty"`
}

func (h *ResourceHandler) CanStartServer(w http.ResponseWriter, r *http.Request) {
	serverName := chi.URLParam(r, "name")

	if serverName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server name required"})
		return
	}

	var ramMB int
	err := h.db.QueryRow("SELECT ram_mb FROM servers WHERE name = ?", serverName).Scan(&ramMB)
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

	ctx := context.Background()
	currentUsage, err := h.docker.GetTotalDemimineMemoryUsage(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to get memory usage: %v", err)})
		return
	}

	containerOverhead := int64(512)
	projectedUsage := currentUsage + int64(ramMB) + containerOverhead
	maxMB := h.cfg.MaxRAMMB

	response := CanStartResponse{
		CanStart:         int64(maxMB) == 0 || projectedUsage <= int64(maxMB),
		CurrentUsageMB:   currentUsage,
		ProjectedUsageMB: projectedUsage,
		MaxMB:            maxMB,
	}

	if !response.CanStart {
		response.Reason = fmt.Sprintf("Projecting %d MB usage would exceed maximum of %d MB", projectedUsage, maxMB)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *ResourceHandler) GetSystemResources(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	containers, err := h.docker.ListManagedContainers(ctx)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to list containers"})
		return
	}

	var containerResources []ContainerResource
	var totalUsage int64

	for _, ctr := range containers {
		containerName := ctr.Names[0]

		stats, err := h.docker.GetContainerMemoryUsage(ctx, containerName)
		if err != nil {
			continue
		}

		var containerType string
		if ctr.Labels["demimine.type"] == "server" {
			containerType = "server"
		} else if ctr.Labels["demimine.type"] == "proxy" {
			containerType = "proxy"
		} else {
			containerType = "manager"
		}

		containerResources = append(containerResources, ContainerResource{
			Name:    containerName,
			Type:    containerType,
			UsageMB: stats,
		})

		totalUsage += stats
	}

	maxMB := h.cfg.MaxRAMMB
	var availableMB int64
	if int64(maxMB) == 0 {
		availableMB = -1
	} else {
		availableMB = int64(maxMB) - totalUsage
	}

	response := SystemResources{
		UsedMB:      totalUsage,
		MaxMB:       maxMB,
		AvailableMB: availableMB,
		Containers:  containerResources,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *ResourceHandler) WaitForContainerRemoval(w http.ResponseWriter, r *http.Request) {
	serverName := chi.URLParam(r, "name")

	if serverName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "server name required"})
		return
	}

	ctx := context.Background()
	timeout := 60 * time.Second

	err := h.docker.WaitForContainerRemoval(ctx, serverName, timeout)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprintf("failed to wait for container removal: %v", err)})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
