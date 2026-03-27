package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/demimine/manager/internal/models"
)

type Manager struct {
	db            *sql.DB
	dataDir       string
	backupDir     string
	serversDir    string
	javaDir       string
	serverHandler ServerHandler
	proxyHandler  ProxyHandler
}

type ServerHandler interface {
	ListAll() ([]models.Server, error)
	StopByID(id int64) error
	StartByID(id int64) error
}

type ProxyHandler interface {
	ListAll() ([]models.Proxy, error)
	StopByID(id int64) error
	StartByID(id int64) error
}

type WSNotifier interface {
	BroadcastBackupStatus(status string)
	BroadcastBackupStatusWithID(backupID int64, status string, message string)
}

func NewManager(db *sql.DB, dataDir, backupDir, serversDir, javaDir string) *Manager {
	return &Manager{
		db:         db,
		dataDir:    dataDir,
		backupDir:  backupDir,
		serversDir: serversDir,
		javaDir:    javaDir,
	}
}

func (m *Manager) SetHandlers(serverHandler ServerHandler, proxyHandler ProxyHandler) {
	m.serverHandler = serverHandler
	m.proxyHandler = proxyHandler
}

func (m *Manager) CreateSnapshot(ctx context.Context, notifier WSNotifier) (*models.Backup, error) {
	pending, err := m.CreatePendingBackup()
	if err != nil {
		return nil, err
	}
	return m.RunBackup(pending.ID, notifier)
}

func (m *Manager) CreatePendingBackup() (*models.Backup, error) {
	now := time.Now()
	dateStr := now.Format("2006-01-02")
	archiveName := fmt.Sprintf("demimine-backup-%s.tar.zst", dateStr)
	archivePath := filepath.Join(m.backupDir, archiveName)

	result, err := m.db.Exec("INSERT INTO backups (created_at, size_bytes, archive_path, status) VALUES (?, ?, ?, ?)",
		now, 0, archivePath, "pending")
	if err != nil {
		return nil, fmt.Errorf("failed to create backup record: %w", err)
	}
	backupID, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("failed to get backup id: %w", err)
	}

	return &models.Backup{
		ID:          backupID,
		CreatedAt:   now,
		ArchivePath: archivePath,
		Status:      "pending",
	}, nil
}

func (m *Manager) RunBackup(backupID int64, notifier WSNotifier) (*models.Backup, error) {
	var archivePath string
	err := m.db.QueryRow("SELECT archive_path FROM backups WHERE id = ?", backupID).Scan(&archivePath)
	if err != nil {
		return nil, fmt.Errorf("backup record not found: %w", err)
	}

	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(backupID, "stopping_servers", "Stopping servers...")
	}

	var flaggedServers []int64
	var flaggedProxies []int64

	if m.serverHandler != nil {
		servers, err := m.serverHandler.ListAll()
		if err != nil {
			m.setBackupStatus(backupID, "failed")
			if notifier != nil {
				notifier.BroadcastBackupStatusWithID(backupID, "failed", fmt.Sprintf("Failed to list servers: %v", err))
			}
			return nil, fmt.Errorf("failed to list servers: %w", err)
		}

		for _, server := range servers {
			if server.Status == "running" {
				if err := m.serverHandler.StopByID(server.ID); err != nil {
					m.setBackupStatus(backupID, "failed")
					if notifier != nil {
						notifier.BroadcastBackupStatusWithID(backupID, "failed", fmt.Sprintf("Failed to stop server %s: %v", server.Name, err))
					}
					return nil, fmt.Errorf("failed to stop server %s: %w", server.Name, err)
				}
			}
			if server.StartOnBoot == 1 {
				flaggedServers = append(flaggedServers, server.ID)
			}
		}
	}

	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(backupID, "stopping_proxies", "Stopping proxies...")
	}

	if m.proxyHandler != nil {
		proxies, err := m.proxyHandler.ListAll()
		if err != nil {
			m.setBackupStatus(backupID, "failed")
			if notifier != nil {
				notifier.BroadcastBackupStatusWithID(backupID, "failed", fmt.Sprintf("Failed to list proxies: %v", err))
			}
			return nil, fmt.Errorf("failed to list proxies: %w", err)
		}

		for _, proxy := range proxies {
			if proxy.Status == "running" {
				if err := m.proxyHandler.StopByID(proxy.ID); err != nil {
					m.setBackupStatus(backupID, "failed")
					if notifier != nil {
						notifier.BroadcastBackupStatusWithID(backupID, "failed", fmt.Sprintf("Failed to stop proxy %s: %v", proxy.Name, err))
					}
					return nil, fmt.Errorf("failed to stop proxy %s: %w", proxy.Name, err)
				}
			}
			if proxy.StartOnBoot == 1 {
				flaggedProxies = append(flaggedProxies, proxy.ID)
			}
		}
	}

	time.Sleep(2 * time.Second)

	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(backupID, "archiving", "Creating archive...")
	}

	tarDirs := "data servers"
	if _, err := os.Stat(m.javaDir); err == nil {
		tarDirs += " java"
	}

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf("tar -cf - -C / %s 2>/dev/null | zstd -f -o %s", tarDirs, archivePath))
	if err := cmd.Run(); err != nil {
		m.setBackupStatus(backupID, "failed")
		if notifier != nil {
			notifier.BroadcastBackupStatusWithID(backupID, "failed", fmt.Sprintf("Failed to create archive: %v", err))
		}
		return nil, fmt.Errorf("failed to create archive: %w", err)
	}

	fileInfo, err := os.Stat(archivePath)
	if err != nil {
		m.setBackupStatus(backupID, "failed")
		if notifier != nil {
			notifier.BroadcastBackupStatusWithID(backupID, "failed", "Failed to get archive size")
		}
		return nil, fmt.Errorf("failed to get archive size: %w", err)
	}

	_, err = m.db.Exec("UPDATE backups SET size_bytes = ?, status = 'complete' WHERE id = ?", fileInfo.Size(), backupID)
	if err != nil {
		return nil, fmt.Errorf("failed to update backup record: %w", err)
	}

	if err := m.CleanupOldBackups(); err != nil {
		fmt.Printf("Warning: failed to cleanup old backups: %v\n", err)
	}

	if (len(flaggedServers) > 0 || len(flaggedProxies) > 0) && notifier != nil {
		notifier.BroadcastBackupStatusWithID(backupID, "restarting", "Restarting servers...")
	}

	if m.serverHandler != nil {
		for _, serverID := range flaggedServers {
			if err := m.serverHandler.StartByID(serverID); err != nil {
				fmt.Printf("Warning: failed to start server %d after backup: %v\n", serverID, err)
			}
		}
	}

	if m.proxyHandler != nil {
		for _, proxyID := range flaggedProxies {
			if err := m.proxyHandler.StartByID(proxyID); err != nil {
				fmt.Printf("Warning: failed to start proxy %d after backup: %v\n", proxyID, err)
			}
		}
	}

	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(backupID, "complete", "Backup completed successfully")
	}

	return m.GetBackup(backupID)
}

func (m *Manager) RestoreSnapshot(ctx context.Context, archivePath string, notifier WSNotifier) error {
	return m.RunRestore(archivePath, notifier)
}

func (m *Manager) RunRestore(archivePath string, notifier WSNotifier) error {
	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(0, "restoring_extracting", "Extracting backup archive...")
	}

	if _, err := os.Stat(archivePath); err != nil {
		if notifier != nil {
			notifier.BroadcastBackupStatusWithID(0, "failed", "Archive not found")
		}
		return fmt.Errorf("archive not found: %w", err)
	}

	tempDir := filepath.Join(m.dataDir, "temp_restore")
	if err := os.RemoveAll(tempDir); err != nil {
		if notifier != nil {
			notifier.BroadcastBackupStatusWithID(0, "failed", "Failed to prepare temp directory")
		}
		return fmt.Errorf("failed to clean temp directory: %w", err)
	}
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		if notifier != nil {
			notifier.BroadcastBackupStatusWithID(0, "failed", "Failed to create temp directory")
		}
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	ctx := context.Background()
	cmd := exec.CommandContext(ctx, "sh", "-c",
		fmt.Sprintf("zstd -d -c %s | tar -xf - -C %s", archivePath, tempDir))
	if err := cmd.Run(); err != nil {
		if notifier != nil {
			notifier.BroadcastBackupStatusWithID(0, "failed", "Failed to extract archive")
		}
		return fmt.Errorf("failed to extract archive: %w", err)
	}

	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(0, "restoring_applying", "Applying restored data...")
	}

	backupData := filepath.Join(tempDir, "data")
	backupServers := filepath.Join(tempDir, "servers")
	backupJava := filepath.Join(tempDir, "java")

	if _, err := os.Stat(backupData); err == nil {
		entries, err := os.ReadDir(backupData)
		if err == nil {
			for _, entry := range entries {
				src := filepath.Join(backupData, entry.Name())
				dst := filepath.Join(m.dataDir, entry.Name())
				os.RemoveAll(dst)
				os.Rename(src, dst)
			}
		}
	}

	if _, err := os.Stat(backupServers); err == nil {
		os.RemoveAll(m.serversDir)
		os.Rename(backupServers, m.serversDir)
	}

	if _, err := os.Stat(backupJava); err == nil {
		os.RemoveAll(m.javaDir)
		os.Rename(backupJava, m.javaDir)
	}

	if notifier != nil {
		notifier.BroadcastBackupStatusWithID(0, "restoring_complete", "Restore complete. Restarting...")
	}

	return nil
}

func (m *Manager) CleanupOldBackups() error {
	retention := m.getRetentionCount()
	if retention <= 0 {
		retention = 2
	}

	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		return fmt.Errorf("failed to list backups: %w", err)
	}

	var backupFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, "demimine-backup-") && strings.HasSuffix(name, ".tar.zst") {
			backupFiles = append(backupFiles, name)
		}
	}

	if len(backupFiles) <= retention {
		return nil
	}

	sort.Strings(backupFiles)

	for _, filename := range backupFiles[:len(backupFiles)-retention] {
		path := filepath.Join(m.backupDir, filename)
		if err := os.Remove(path); err != nil {
			fmt.Printf("Warning: failed to delete old backup %s: %v\n", filename, err)
			continue
		}

		_, err := m.db.Exec("DELETE FROM backups WHERE archive_path = ?", path)
		if err != nil {
			fmt.Printf("Warning: failed to delete backup record for %s: %v\n", filename, err)
		}
	}

	return nil
}

func (m *Manager) ShouldRunBackup() bool {
	interval := m.getBackupInterval()
	if interval <= 0 {
		return false
	}

	var lastDate sql.NullString
	m.db.QueryRow("SELECT created_at FROM backups WHERE status = 'complete' ORDER BY created_at DESC LIMIT 1").Scan(&lastDate)

	if !lastDate.Valid || lastDate.String == "" {
		return true
	}

	lastTime, err := time.Parse(time.RFC3339Nano, lastDate.String)
	if err != nil {
		return true
	}

	daysSince := int(time.Since(lastTime).Hours() / 24)
	return daysSince >= interval
}

func (m *Manager) AlreadyCheckedToday() bool {
	checkDate := m.getLastCheckDate()
	return checkDate == time.Now().Format("2006-01-02")
}

func (m *Manager) MarkCheckedToday() {
	m.setLastCheckDate(time.Now().Format("2006-01-02"))
}

func (m *Manager) getLastCheckDate() string {
	var date string
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'last_backup_check_date'").Scan(&date)
	return date
}

func (m *Manager) setLastCheckDate(date string) error {
	_, err := m.db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('last_backup_check_date', ?)", date)
	return err
}

func (m *Manager) ListBackups() ([]models.Backup, error) {
	var backups []models.Backup
	var orphanIDs []int64

	rows, err := m.db.Query("SELECT id, created_at, size_bytes, archive_path, status FROM backups ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var b models.Backup
		if err := rows.Scan(&b.ID, &b.CreatedAt, &b.SizeBytes, &b.ArchivePath, &b.Status); err != nil {
			return nil, err
		}

		if _, err := os.Stat(b.ArchivePath); os.IsNotExist(err) {
			orphanIDs = append(orphanIDs, b.ID)
			continue
		}

		backups = append(backups, b)
	}

	for _, id := range orphanIDs {
		m.db.Exec("DELETE FROM backups WHERE id = ?", id)
	}

	return backups, nil
}

func (m *Manager) GetBackup(id int64) (*models.Backup, error) {
	var b models.Backup
	err := m.db.QueryRow("SELECT id, created_at, size_bytes, archive_path, status FROM backups WHERE id = ?", id).
		Scan(&b.ID, &b.CreatedAt, &b.SizeBytes, &b.ArchivePath, &b.Status)
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (m *Manager) DeleteBackup(id int64) error {
	backup, err := m.GetBackup(id)
	if err != nil {
		return err
	}

	if err := os.Remove(backup.ArchivePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete backup file: %w", err)
	}

	_, err = m.db.Exec("DELETE FROM backups WHERE id = ?", id)
	return err
}

func (m *Manager) setBackupStatus(id int64, status string) error {
	_, err := m.db.Exec("UPDATE backups SET status = ? WHERE id = ?", status, id)
	return err
}

func (m *Manager) getBackupTime() string {
	var time string
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_time'").Scan(&time)
	if time == "" {
		return "03:00"
	}
	return time
}

func (m *Manager) getBackupInterval() int {
	var interval int
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'backup_interval_days'").Scan(&interval)
	if interval <= 0 {
		return 3
	}
	return interval
}

func (m *Manager) getRetentionCount() int {
	var count int
	m.db.QueryRow("SELECT value FROM settings WHERE key = 'retention_count'").Scan(&count)
	if count <= 0 {
		return 2
	}
	return count
}

func (m *Manager) GetSettings() map[string]string {
	settings := make(map[string]string)

	rows, _ := m.db.Query("SELECT key, value FROM settings WHERE key IN ('backup_time', 'backup_interval_days', 'retention_count')")
	defer rows.Close()

	for rows.Next() {
		var key, value string
		rows.Scan(&key, &value)
		settings[key] = value
	}

	if _, ok := settings["backup_time"]; !ok {
		settings["backup_time"] = "03:00"
	}
	if _, ok := settings["backup_interval_days"]; !ok {
		settings["backup_interval_days"] = "3"
	}
	if _, ok := settings["retention_count"]; !ok {
		settings["retention_count"] = "2"
	}

	return settings
}
