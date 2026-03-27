package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func setupTestEnv(t *testing.T) (*sql.DB, string, string, string, string) {
	base := t.TempDir()
	dataDir := base
	backupDir := filepath.Join(base, "backups")
	serversDir := filepath.Join(base, "servers")
	javaDir := filepath.Join(base, "java")

	for _, dir := range []string{backupDir, serversDir, javaDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("Failed to create dir %s: %v", dir, err)
		}
	}

	dbPath := filepath.Join(dataDir, "demimine.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS backups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			size_bytes INTEGER DEFAULT 0,
			archive_path TEXT,
			status TEXT DEFAULT 'pending'
		);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);
		CREATE TABLE IF NOT EXISTS servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			version TEXT NOT NULL,
			status TEXT DEFAULT 'stopped',
			start_on_boot INTEGER DEFAULT 0
		);
		CREATE TABLE IF NOT EXISTS proxies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			host_port INTEGER NOT NULL,
			status TEXT DEFAULT 'stopped',
			start_on_boot INTEGER DEFAULT 0
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create tables: %v", err)
	}

	serverDir := filepath.Join(serversDir, "test-server")
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		t.Fatalf("Failed to create server dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "server.properties"), []byte("motd=Test Server\nserver-port=25565"), 0644); err != nil {
		t.Fatalf("Failed to create server.properties: %v", err)
	}
	if err := os.WriteFile(filepath.Join(serverDir, "world", "level.dat"), []byte("test world data"), 0644); err != nil {
		os.MkdirAll(filepath.Join(serverDir, "world"), 0755)
		os.WriteFile(filepath.Join(serverDir, "world", "level.dat"), []byte("test world data"), 0644)
	}

	return db, dataDir, backupDir, serversDir, javaDir
}

func TestFunctional_BackupAndRestore(t *testing.T) {
	db, dataDir, backupDir, serversDir, javaDir := setupTestEnv(t)
	defer db.Close()

	manager := NewManager(db, dataDir, backupDir, serversDir, javaDir)

	archivePath := filepath.Join(backupDir, "test-backup.tar.zst")
	tarArgs := "-C " + dataDir + " servers demimine.db"
	cmd := exec.Command("sh", "-c", fmt.Sprintf("tar -cf - %s | zstd -f -o %s", tarArgs, archivePath))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create backup archive: %v", err)
	}

	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Fatal("Backup archive was not created")
	}

	info, err := os.Stat(archivePath)
	if err != nil {
		t.Fatalf("Failed to stat archive: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("Backup archive is empty")
	}
	t.Logf("Backup archive created: %s (%d bytes)", archivePath, info.Size())

	listCmd := exec.Command("sh", "-c", fmt.Sprintf("zstd -d -c %s | tar -tf -", archivePath))
	output, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to list archive contents: %v\n%s", err, output)
	}
	t.Logf("Archive contents:\n%s", string(output))

	os.RemoveAll(serversDir)
	os.Remove(filepath.Join(dataDir, "demimine.db"))

	err = manager.RestoreSnapshot(context.Background(), archivePath, nil)
	if err != nil {
		t.Fatalf("RestoreSnapshot failed: %v", err)
	}

	propsPath := filepath.Join(serversDir, "test-server", "server.properties")
	if _, err := os.Stat(propsPath); os.IsNotExist(err) {
		t.Fatal("server.properties was not restored")
	}
	content, err := os.ReadFile(propsPath)
	if err != nil {
		t.Fatalf("Failed to read restored server.properties: %v", err)
	}
	if string(content) != "motd=Test Server\nserver-port=25565" {
		t.Errorf("server.properties content mismatch: %s", string(content))
	}

	t.Log("Backup and restore cycle completed successfully")

	os.Remove(archivePath)
}

func TestFunctional_CleanupKeepsOnlyTwo(t *testing.T) {
	db, dataDir, backupDir, serversDir, javaDir := setupTestEnv(t)
	defer db.Close()

	manager := NewManager(db, dataDir, backupDir, serversDir, javaDir)

	for i := 1; i <= 5; i++ {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		filename := "demimine-backup-" + date + ".tar.zst"
		path := filepath.Join(backupDir, filename)

		tarArgs := "-C " + dataDir + " servers demimine.db"
		cmd := exec.Command("sh", "-c", fmt.Sprintf("tar -cf - %s | zstd -f -o %s", tarArgs, path))
		if err := cmd.Run(); err != nil {
			t.Fatalf("Failed to create backup %s: %v", filename, err)
		}

		_, err := db.Exec("INSERT INTO backups (created_at, size_bytes, archive_path, status) VALUES (?, ?, ?, 'complete')",
			time.Now().AddDate(0, 0, -i), 1000, path)
		if err != nil {
			t.Fatalf("Failed to insert backup record: %v", err)
		}
	}

	entries, _ := os.ReadDir(backupDir)
	t.Logf("Before cleanup: %d backup files", len(entries))

	err := manager.CleanupOldBackups()
	if err != nil {
		t.Fatalf("CleanupOldBackups failed: %v", err)
	}

	entries, _ = os.ReadDir(backupDir)
	backupCount := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".zst" {
			backupCount++
		}
	}

	if backupCount != 2 {
		t.Errorf("Expected 2 backups after cleanup, got %d", backupCount)
	}

	var dbCount int
	db.QueryRow("SELECT COUNT(*) FROM backups").Scan(&dbCount)
	if dbCount != 2 {
		t.Errorf("Expected 2 backup records in db, got %d", dbCount)
	}

	t.Log("Cleanup verified: only 2 most recent backups remain")

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".zst" {
			os.Remove(filepath.Join(backupDir, entry.Name()))
		}
	}
}

func TestFunctional_ScheduledBackupLogic(t *testing.T) {
	db, dataDir, backupDir, serversDir, javaDir := setupTestEnv(t)
	defer db.Close()

	manager := NewManager(db, dataDir, backupDir, serversDir, javaDir)

	if !manager.ShouldRunBackup() {
		t.Error("Should run backup when no previous backup exists")
	}

	_, err := db.Exec("INSERT INTO backups (created_at, size_bytes, archive_path, status) VALUES (?, ?, ?, 'complete')",
		time.Now(), 1000, filepath.Join(backupDir, "test.tar.zst"))
	if err != nil {
		t.Fatalf("Failed to insert backup record: %v", err)
	}

	if manager.ShouldRunBackup() {
		t.Error("Should NOT run backup when backup was just created")
	}

	db.Exec("DELETE FROM backups")

	_, err = db.Exec("INSERT INTO backups (created_at, size_bytes, archive_path, status) VALUES (?, ?, ?, 'complete')",
		time.Now().AddDate(0, 0, -4), 1000, filepath.Join(backupDir, "test-old.tar.zst"))
	if err != nil {
		t.Fatalf("Failed to insert old backup record: %v", err)
	}

	if !manager.ShouldRunBackup() {
		t.Error("Should run backup when most recent backup is 4 days old (interval=3)")
	}

	if manager.AlreadyCheckedToday() {
		t.Error("Should NOT have checked today yet")
	}

	manager.MarkCheckedToday()

	if !manager.AlreadyCheckedToday() {
		t.Error("Should have checked today after MarkCheckedToday")
	}

	t.Log("Scheduled backup logic verified")
}

func TestFunctional_SettingsPersistence(t *testing.T) {
	db, dataDir, backupDir, serversDir, javaDir := setupTestEnv(t)
	defer db.Close()

	manager := NewManager(db, dataDir, backupDir, serversDir, javaDir)

	settings := manager.GetSettings()
	if settings["backup_time"] != "03:00" {
		t.Errorf("Default backup_time should be '03:00', got %s", settings["backup_time"])
	}
	if settings["backup_interval_days"] != "3" {
		t.Errorf("Default backup_interval_days should be '3', got %s", settings["backup_interval_days"])
	}
	if settings["retention_count"] != "2" {
		t.Errorf("Default retention_count should be '2', got %s", settings["retention_count"])
	}

	_, err := db.Exec("INSERT INTO settings (key, value) VALUES ('backup_time', '05:30')")
	if err != nil {
		t.Fatalf("Failed to set backup_time: %v", err)
	}
	_, err = db.Exec("INSERT INTO settings (key, value) VALUES ('backup_interval_days', '7')")
	if err != nil {
		t.Fatalf("Failed to set interval: %v", err)
	}
	_, err = db.Exec("INSERT INTO settings (key, value) VALUES ('retention_count', '5')")
	if err != nil {
		t.Fatalf("Failed to set retention count: %v", err)
	}

	settings = manager.GetSettings()
	if settings["backup_time"] != "05:30" {
		t.Errorf("backup_time should be '05:30', got %s", settings["backup_time"])
	}
	if settings["backup_interval_days"] != "7" {
		t.Errorf("backup_interval_days should be '7', got %s", settings["backup_interval_days"])
	}
	if settings["retention_count"] != "5" {
		t.Errorf("retention_count should be '5', got %s", settings["retention_count"])
	}

	t.Log("Settings persistence verified")
}
