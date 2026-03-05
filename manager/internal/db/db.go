package db

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

var (
	db   *sql.DB
	once sync.Once
)

func Connect(databaseURL string) (*sql.DB, error) {
	var err error
	once.Do(func() {
		if databaseURL == "" {
			databaseURL = "file:/data/demimine.db?cache=shared&_journal_mode=WAL&_busy_timeout=5000"
		}
		
		db, err = sql.Open("sqlite", databaseURL)
		if err != nil {
			return
		}
		
		db.SetMaxOpenConns(1)
		db.SetMaxIdleConns(1)
		db.SetConnMaxLifetime(0)
		
		if err = db.Ping(); err != nil {
			return
		}
	})
	return db, err
}

func Get() *sql.DB {
	if db == nil {
		panic("database not connected")
	}
	return db
}

func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

func RunMigrations(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS proxies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			host_port INTEGER NOT NULL UNIQUE,
			status TEXT DEFAULT 'stopped',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		
		`CREATE TABLE IF NOT EXISTS servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			type TEXT NOT NULL,
			version TEXT NOT NULL,
			proxy_id INTEGER,
			host_port INTEGER,
			ram_mb INTEGER NOT NULL,
			domain TEXT,
			backup_interval_days INTEGER DEFAULT 0,
			auto_shutdown_minutes INTEGER DEFAULT 15,
			scheduled_start TEXT,
			scheduled_stop TEXT,
			status TEXT DEFAULT 'stopped',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (proxy_id) REFERENCES proxies(id) ON DELETE SET NULL
		)`,
		
		`CREATE TABLE IF NOT EXISTS players (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			uuid TEXT NOT NULL,
			server_id INTEGER NOT NULL,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
		)`,
		
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key_hash TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		)`,
		
		`CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
		
		`CREATE TABLE IF NOT EXISTS java_versions (
			version TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			downloaded_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		
		`CREATE TABLE IF NOT EXISTS backups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			path TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			size_bytes INTEGER NOT NULL,
			FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
		)`,
		
		`CREATE TABLE IF NOT EXISTS crash_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			acknowledged INTEGER DEFAULT 0,
			FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
		)`,
		
		`CREATE TABLE IF NOT EXISTS command_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			server_id INTEGER NOT NULL,
			command TEXT NOT NULL,
			executed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
		)`,
		
		`CREATE TABLE IF NOT EXISTS admin_auth (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL,
			password_hash TEXT NOT NULL,
			setup_complete INTEGER DEFAULT 0
		)`,
	}
	
	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_proxies_status ON proxies(status)`,
		`CREATE INDEX IF NOT EXISTS idx_servers_status ON servers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_servers_proxy ON servers(proxy_id)`,
		`CREATE INDEX IF NOT EXISTS idx_players_server ON players(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_players_uuid ON players(uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_backups_server ON backups(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_crash_logs_server ON crash_logs(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_command_history_server ON command_history(server_id)`,
	}
	
	for _, index := range indexes {
		if _, err := db.Exec(index); err != nil {
			return fmt.Errorf("index creation failed: %w", err)
		}
	}
	
	return nil
}
