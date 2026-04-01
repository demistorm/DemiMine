package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/demimine/manager/internal/java"
	"github.com/demimine/manager/internal/mc"
	"github.com/go-chi/chi/v5"
)

type VersionsHandler struct{}

func NewVersionsHandler() *VersionsHandler {
	return &VersionsHandler{}
}

func (h *VersionsHandler) List(w http.ResponseWriter, r *http.Request) {
	serverType := chi.URLParam(r, "type")
	includeAll := r.URL.Query().Get("all") == "true"

	versions, err := mc.GetVersions(serverType, includeAll)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"versions": versions,
	})
}

type JavaHandler struct{}

func NewJavaHandler() *JavaHandler {
	return &JavaHandler{}
}

func (h *JavaHandler) List(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": []string{"8", "17", "21", "25"},
	})
}

func (h *JavaHandler) GetRequired(w http.ResponseWriter, r *http.Request) {
	version := r.URL.Query().Get("mc_version")
	if version == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"error": "mc_version query parameter required",
		})
		return
	}

	serverType := r.URL.Query().Get("server_type")
	javaVersion := java.GetRequiredJavaVersionForServerType(serverType, version)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"java_version": javaVersion,
	})
}
