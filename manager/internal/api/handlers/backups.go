package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/demimine/manager/internal/api/middleware"
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
	n.BroadcastBackupStatusWithID(0, status, "")
}

func (n *WSNotifier) BroadcastBackupStatusWithID(backupID int64, status string, message string) {
	msg := map[string]interface{}{
		"type":      "backup_status",
		"status":    status,
		"backup_id": backupID,
		"message":   message,
		"timestamp": time.Now().Unix(),
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
		middleware.WriteJSONInternalError(w, err, "failed to list backups")
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
	pending, err := h.manager.CreatePendingBackup()
	if err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to create backup")
		return
	}

	go func() {
		notifier := &WSNotifier{hub: h.hub}
		if _, err := h.manager.RunBackup(pending.ID, notifier); err != nil {
			log.Printf("Backup %d failed: %v", pending.ID, err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"backup_id": pending.ID,
		"status":    "pending",
	})
}

func (h *BackupsHandler) Download(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		middleware.WriteJSONError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	bk, err := h.manager.GetBackup(id)
	if err != nil {
		middleware.WriteJSONError(w, http.StatusNotFound, "backup not found")
		return
	}

	http.ServeFile(w, r, bk.ArchivePath)
}

func (h *BackupsHandler) RestoreFromUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		middleware.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	file, _, err := r.FormFile("backup")
	if err != nil {
		middleware.WriteJSONError(w, http.StatusBadRequest, "no backup file provided")
		return
	}
	defer file.Close()

	tempPath := fmt.Sprintf("/tmp/demimine-restore-%d.tar.zst", time.Now().Unix())
	tempFile, err := createTempFile(tempPath, file)
	if err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to save uploaded file")
		return
	}
	tempFile.Close()

	go func() {
		notifier := &WSNotifier{hub: h.hub}
		if err := h.manager.RunRestore(tempPath, notifier); err != nil {
			log.Printf("Restore from upload failed: %v", err)
		}
		os.Remove(tempPath)
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "restoring",
		"message": "Restore started. System will restart when complete.",
	})
}

func (h *BackupsHandler) RestoreFromDisk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		middleware.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		middleware.WriteJSONError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	bk, err := h.manager.GetBackup(id)
	if err != nil {
		middleware.WriteJSONError(w, http.StatusNotFound, "backup not found")
		return
	}

	go func() {
		notifier := &WSNotifier{hub: h.hub}
		if err := h.manager.RunRestore(bk.ArchivePath, notifier); err != nil {
			log.Printf("Restore from disk failed: %v", err)
		}
	}()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"status":  "restoring",
		"message": "Restore started. System will restart when complete.",
	})
}

func (h *BackupsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		middleware.WriteJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		middleware.WriteJSONError(w, http.StatusBadRequest, "invalid backup id")
		return
	}

	if err := h.manager.DeleteBackup(id); err != nil {
		middleware.WriteJSONInternalError(w, err, "failed to delete backup")
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
