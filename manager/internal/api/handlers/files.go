package handlers

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/demimine/manager/internal/config"
	"github.com/go-chi/chi/v5"
)

type FileUploadHandler struct {
	db  *sql.DB
	cfg *config.Config
}

func NewFileUploadHandler(db *sql.DB, cfg *config.Config) *FileUploadHandler {
	return &FileUploadHandler{db: db, cfg: cfg}
}

func (h *FileUploadHandler) Upload(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid server id"})
		return
	}

	var serverName string
	err = h.db.QueryRow("SELECT name FROM servers WHERE id = ?", id).Scan(&serverName)
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

	if err := r.ParseMultipartForm(100 << 20); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to parse multipart form"})
		return
	}

	relativePath := r.URL.Query().Get("path")
	if relativePath == "" {
		relativePath = "/"
	}

	serverPath := filepath.Join(h.cfg.ServersDir, serverName)
	targetDir := filepath.Join(serverPath, filepath.Clean("/"+relativePath))

	if !strings.HasPrefix(targetDir, serverPath) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "access denied"})
		return
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to create target directory"})
		return
	}

	var uploaded []string

	for _, files := range r.MultipartForm.File {
		for _, fileHeader := range files {
			uploaded = append(uploaded, h.processUploadedFile(fileHeader, targetDir)...)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"uploaded": uploaded,
	})
}

func (h *FileUploadHandler) processUploadedFile(fileHeader *multipart.FileHeader, targetDir string) []string {
	var uploaded []string

	file, err := fileHeader.Open()
	if err != nil {
		return uploaded
	}
	defer file.Close()

	if strings.HasSuffix(strings.ToLower(fileHeader.Filename), ".zip") {
		tempPath := filepath.Join(targetDir, ".upload_temp.zip")
		if err := saveMultipartFile(file, tempPath); err != nil {
			return uploaded
		}
		defer os.Remove(tempPath)

		extracted := extractZip(tempPath, targetDir)
		uploaded = append(uploaded, extracted...)
	} else {
		destPath := filepath.Join(targetDir, fileHeader.Filename)
		if err := saveMultipartFile(file, destPath); err == nil {
			uploaded = append(uploaded, fileHeader.Filename)
		}
	}

	return uploaded
}

func saveMultipartFile(src multipart.File, dest string) error {
	dst, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func extractZip(zipPath, destDir string) []string {
	var extracted []string

	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return extracted
	}
	defer reader.Close()

	for _, file := range reader.File {
		destPath := filepath.Join(destDir, file.Name)

		if !strings.HasPrefix(filepath.Clean(destPath), filepath.Clean(destDir)) {
			continue
		}

		if file.FileInfo().IsDir() {
			os.MkdirAll(destPath, file.Mode())
			continue
		}

		os.MkdirAll(filepath.Dir(destPath), 0755)

		destFile, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			continue
		}

		srcFile, err := file.Open()
		if err != nil {
			destFile.Close()
			continue
		}

		io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()

		extracted = append(extracted, file.Name)
	}

	return extracted
}
