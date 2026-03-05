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

	versions, err := mc.GetVersions(serverType)
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
	available, err := java.GetAvailableJavaVersions()
	if err != nil {
		available = []string{"8", "11", "17", "21"}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"available": available,
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

	javaVersion := java.GetRequiredJavaVersion(version)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"java_version": javaVersion,
	})
}
