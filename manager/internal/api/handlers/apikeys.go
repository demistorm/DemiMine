package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/demimine/manager/internal/api/middleware"
	"github.com/go-chi/chi/v5"
)

type APIKeyHandler struct {
	db *sql.DB
}

func NewAPIKeyHandler(db *sql.DB) *APIKeyHandler {
	return &APIKeyHandler{db: db}
}

type APIKeyCreateRequest struct {
	Name string `json:"name" binding:"required"`
}

type APIKeyResponse struct {
	ID         int64   `json:"id"`
	Name       string  `json:"name"`
	Key        string  `json:"key"`
	CreatedAt  string  `json:"created_at"`
	LastUsedAt *string `json:"last_used_at"`
}

func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.db.Query(`
		SELECT id, name, created_at, last_used_at
		FROM api_keys
		ORDER BY created_at DESC
	`)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to list API keys"})
		return
	}
	defer rows.Close()

	keys := []APIKeyResponse{}
	for rows.Next() {
		var k APIKeyResponse
		var lastUsedAt sql.NullString
		err := rows.Scan(&k.ID, &k.Name, &k.CreatedAt, &lastUsedAt)
		if err != nil {
			continue
		}
		k.LastUsedAt = nullStringToPtr(lastUsedAt)
		keys = append(keys, k)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(keys)
}

func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req APIKeyCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	key := generateRandomKey()
	keyHash := middleware.HashAPIKey(key)

	result, err := h.db.Exec(`
		INSERT INTO api_keys (key_hash, name, created_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
	`, keyHash, req.Name)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create API key"})
		return
	}

	id, _ := result.LastInsertId()
	var createdAt string
	_ = h.db.QueryRow("SELECT datetime(created_at) FROM api_keys WHERE id = ?", id).Scan(&createdAt)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIKeyResponse{
		ID:         id,
		Name:       req.Name,
		Key:        key,
		CreatedAt:  createdAt,
		LastUsedAt: nil,
	})
}

func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid API key ID"})
		return
	}

	result, err := h.db.Exec("DELETE FROM api_keys WHERE id = ?", id)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to delete API key"})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "API key not found"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

func generateRandomKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.URLEncoding.EncodeToString(b)
}
