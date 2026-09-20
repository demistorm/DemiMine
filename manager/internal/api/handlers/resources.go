package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/demimine/manager/internal/api/middleware"
	"github.com/demimine/manager/internal/config"
	"github.com/demimine/manager/internal/docker"
	"github.com/docker/docker/api/types"
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

func (h *ResourceHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()
	status := "ok"
	httpStatus := http.StatusOK
	checks := map[string]string{}

	if err := h.db.PingContext(ctx); err != nil {
		status = "unhealthy"
		httpStatus = http.StatusServiceUnavailable
		checks["database"] = "unreachable"
	} else {
		checks["database"] = "ok"
	}

	if h.docker != nil {
		if err := h.docker.Ping(ctx); err != nil {
			status = "unhealthy"
			httpStatus = http.StatusServiceUnavailable
			checks["docker"] = "unreachable"
		} else {
			checks["docker"] = "ok"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": status,
		"checks": checks,
	})
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

// ProjectRAMUsage returns current live DemiMine memory usage plus what a
// server with the given heap would add (heap + flat overhead). extraMB lets
// callers account for containers already committed but not yet reporting
// usage (e.g. concurrent boot starts).
func ProjectRAMUsage(dockerClient *docker.Client, ramMB int, extraMB int64) (current int64, projected int64, err error) {
	current, err = dockerClient.GetTotalDemimineMemoryUsage(context.Background())
	if err != nil {
		return 0, 0, err
	}
	projected = current + extraMB + docker.ServerMemoryLimitMB(ramMB)
	return current, projected, nil
}

// CheckRAMBudget enforces DEMIMINE_MAX_RAM_MB (0 = unlimited) before a server
// is automatically started. Used by the scheduler, post-backup restarts, and
// start_on_boot so background starts stay within the budget. Manual UI starts
// deliberately skip this — admins get to override.
func CheckRAMBudget(dockerClient *docker.Client, cfg *config.Config, ramMB int, extraMB int64) (bool, string) {
	maxMB := int64(cfg.MaxRAMMB)
	if maxMB == 0 {
		return true, ""
	}

	_, projected, err := ProjectRAMUsage(dockerClient, ramMB, extraMB)
	if err != nil {
		return false, fmt.Sprintf("failed to get current memory usage: %v", err)
	}
	if projected > maxMB {
		return false, fmt.Sprintf("projecting %d MB usage would exceed maximum of %d MB", projected, maxMB)
	}
	return true, ""
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
	err := h.db.QueryRow("SELECT ram_mb FROM servers WHERE sanitized_name = ?", serverName).Scan(&ramMB)
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

	currentUsage, projectedUsage, err := ProjectRAMUsage(h.docker, ramMB, 0)
	if err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to get memory usage")
		return
	}

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

	type containerResult struct {
		name    string
		ct      ContainerResource
		usageMB int64
	}

	resultChan := make(chan containerResult, len(containers))
	var wg sync.WaitGroup

	for _, ctr := range containers {
		wg.Add(1)
		go func(c types.Container) {
			defer wg.Done()
			containerName := c.Names[0]

			stats, err := h.docker.GetContainerMemoryUsage(ctx, containerName)
			if err != nil {
				return
			}

			var containerType string
			if c.Labels["demimine.type"] == "server" {
				containerType = "server"
			} else if c.Labels["demimine.type"] == "proxy" {
				containerType = "proxy"
			} else {
				containerType = "manager"
			}

			resultChan <- containerResult{
				name: containerName,
				ct: ContainerResource{
					Name:    containerName,
					Type:    containerType,
					UsageMB: stats,
				},
				usageMB: stats,
			}
		}(ctr)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	var containerResources []ContainerResource
	var totalUsage int64
	for result := range resultChan {
		containerResources = append(containerResources, result.ct)
		totalUsage += result.usageMB
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
		middleware.WriteJSONInternalError(w, err, "failed to wait for container removal")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}
