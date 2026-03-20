package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/demimine/manager/internal/backup"
	"github.com/demimine/manager/internal/docker"
	"github.com/demimine/manager/internal/websocket"
)

type BackupsHandler struct {
	db             *sql.DB
	manager        *backup.Manager
	serverHandler  *ServerHandler
	proxyHandler   *ProxyHandler
	hub            *websocket.Hub
	consoleManager *docker.ConsoleManager
}

func NewBackupsHandler(db *sql.DB, manager *backup.Manager, serverHandler *ServerHandler, proxyHandler *ProxyHandler, hub *websocket.Hub, consoleManager *docker.ConsoleManager) *BackupsHandler {
	return &BackupsHandler{
		db:             db,
		manager:        manager,
		serverHandler:  serverHandler,
		proxyHandler:   proxyHandler,
		hub:            hub,
		consoleManager: consoleManager,
	}
}

type WSNotifier struct {
	hub *websocket.Hub
}

func (n *WSNotifier) BroadcastBackupStatus(status string) {
	msg := map[string]interface{}{
		"type":   "backup_status",
		"status": status,
	}
	data, _ := json.Marshal(msg)
	n.hub.Broadcast(data)
}

type BackupResponse struct {
	ID          int64  `json:"id"`
	CreatedAt   string `json:"created_at"`
	SizeBytes   int64  `json:"size_bytes"`
	SizeHuman   string `json:"size_human"`
	ArchivePath string `json:"archive_path"`
	Status      string `json:"status"`
}

func (h *BackupsHandler) List(w http.ResponseWriter, r *http.Request) {
	backups, err := h.manager.ListBackups()
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to list backups: %v"}`, err), http.StatusInternalServerError)
		return
	}

	response := []BackupResponse{}
	for _, b := range backups {
		response = append(response, BackupResponse{
			ID:          b.ID,
			CreatedAt:   b.CreatedAt.Format(time.RFC3339),
			SizeBytes:   b.SizeBytes,
			SizeHuman:   formatBytes(b.SizeBytes),
			ArchivePath: b.ArchivePath,
			Status:      b.Status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (h *BackupsHandler) Create(w http.ResponseWriter, r *http.Request) {
	notifier := &WSNotifier{hub: h.hub}

	backup, err := h.manager.CreateSnapshot(context.Background(), notifier)
	if err != nil {
		notifier.BroadcastBackupStatus("failed")
		http.Error(w, fmt.Sprintf(`{"error":"failed to create backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"backup_id": backup.ID,
	})
}

func (h *BackupsHandler) Download(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid backup id"}`, http.StatusBadRequest)
		return
	}

	backup, err := h.manager.GetBackup(id)
	if err != nil {
		http.Error(w, `{"error":"backup not found"}`, http.StatusNotFound)
		return
	}

	http.ServeFile(w, r, backup.ArchivePath)
}

func (h *BackupsHandler) RestoreFromUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	file, header, err := r.FormFile("backup")
	if err != nil {
		http.Error(w, `{"error":"no backup file provided"}`, http.StatusBadRequest)
		return
	}
	defer file.Close()

	tempPath := fmt.Sprintf("/tmp/demimine-restore-%d.tar.zst", time.Now().Unix())
	tempFile, err := createTempFile(tempPath, file)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to save uploaded file: %v"}`, err), http.StatusInternalServerError)
		return
	}
	defer tempFile.Close()

	notifier := &WSNotifier{hub: h.hub}

	if err := h.manager.RestoreSnapshot(context.Background(), tempPath, notifier); err != nil {
		notifier.BroadcastBackupStatus("failed")
		http.Error(w, fmt.Sprintf(`{"error":"failed to restore backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"message":  "Backup restored successfully. The manager will restart shortly.",
		"filename": header.Filename,
	})
}

func (h *BackupsHandler) RestoreFromDisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid backup id"}`, http.StatusBadRequest)
		return
	}

	backup, err := h.manager.GetBackup(id)
	if err != nil {
		http.Error(w, `{"error":"backup not found"}`, http.StatusNotFound)
		return
	}

	notifier := &WSNotifier{hub: h.hub}

	if err := h.manager.RestoreSnapshot(context.Background(), backup.ArchivePath, notifier); err != nil {
		notifier.BroadcastBackupStatus("failed")
		http.Error(w, fmt.Sprintf(`{"error":"failed to restore backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Backup restored successfully. The manager will restart shortly.",
	})
}

func (h *BackupsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, `{"error":"invalid backup id"}`, http.StatusBadRequest)
		return
	}

	if err := h.manager.DeleteBackup(id); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to delete backup: %v"}`, err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func createTempFile(path string, src io.Reader) (*os.File, error) {
	dst, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	_, err = io.Copy(dst, src)
	if err != nil {
		dst.Close()
		os.Remove(path)
		return nil, err
	}
	return dst, nil
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
