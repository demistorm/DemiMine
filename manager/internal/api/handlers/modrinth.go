package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/demimine/manager/internal/modrinth"
	"github.com/go-chi/chi/v5"
)

type ModrinthHandler struct {
	client *modrinth.Client
}

func NewModrinthHandler() *ModrinthHandler {
	return &ModrinthHandler{
		client: modrinth.NewClient(),
	}
}

func (h *ModrinthHandler) Search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("query")
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}
	offset := 0
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	loadersRaw := r.URL.Query().Get("loaders")
	var loaders []string
	if loadersRaw != "" {
		loaders = strings.Split(loadersRaw, ",")
	}
	gameVersion := r.URL.Query().Get("game_version")

	var facets [][]string
	if len(loaders) > 0 || gameVersion != "" {
		facets = modrinth.BuildPluginFacets(loaders, gameVersion)
	} else {
		facets = [][]string{{"project_type:plugin"}}
	}

	result, err := h.client.SearchPlugins(modrinth.SearchParams{
		Query:  query,
		Limit:  limit,
		Offset: offset,
		Facets: facets,
	})
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *ModrinthHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	slugOrID := chi.URLParam(r, "slug")
	if slugOrID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "slug required"})
		return
	}

	project, err := h.client.GetProject(slugOrID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(project)
}

func (h *ModrinthHandler) GetVersions(w http.ResponseWriter, r *http.Request) {
	slugOrID := chi.URLParam(r, "slug")
	if slugOrID == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "slug required"})
		return
	}

	var gameVersions []string
	if gv := r.URL.Query().Get("game_versions"); gv != "" {
		if err := json.Unmarshal([]byte(gv), &gameVersions); err != nil {
			gameVersions = nil
		}
	}

	var loaders []string
	if l := r.URL.Query().Get("loaders"); l != "" {
		if err := json.Unmarshal([]byte(l), &loaders); err != nil {
			loaders = nil
		}
	}

	versions, err := h.client.GetProjectVersions(slugOrID, gameVersions, loaders)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(versions)
}
