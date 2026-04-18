package db

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/demimine/manager/internal/util"
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

		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(2)
		db.SetConnMaxLifetime(5 * time.Minute)

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
			forwarding_secret TEXT NOT NULL,
			status TEXT DEFAULT 'stopped',
			canvas_x INTEGER DEFAULT 0,
			canvas_y INTEGER DEFAULT 0,
			motd_line1 TEXT,
			motd_line2 TEXT,
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
			canvas_x INTEGER DEFAULT 0,
			canvas_y INTEGER DEFAULT 0,
			minimotd_line1 TEXT,
			minimotd_line2 TEXT,
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

		`CREATE TABLE IF NOT EXISTS backups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			size_bytes INTEGER NOT NULL,
			archive_path TEXT NOT NULL,
			status TEXT DEFAULT 'complete'
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

		`CREATE TABLE IF NOT EXISTS installed_plugins (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_type TEXT NOT NULL,
			target_id INTEGER NOT NULL,
			project_id TEXT NOT NULL,
			project_slug TEXT NOT NULL,
			project_name TEXT NOT NULL,
			version_id TEXT NOT NULL,
			version_number TEXT NOT NULL,
			filename TEXT NOT NULL,
			file_hash TEXT NOT NULL,
			installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (target_id) REFERENCES servers(id) ON DELETE CASCADE
		)`,
	}

	for _, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_version (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		return fmt.Errorf("failed to create schema_version table: %w", err)
	}

	currentVersion := getSchemaVersion(db)

	alterMigrations := map[int][]struct {
		sql  string
		desc string
	}{
		1: {
			{`ALTER TABLE proxies ADD COLUMN forwarding_secret TEXT`, "add proxies.forwarding_secret"},
			{`ALTER TABLE proxies ADD COLUMN canvas_x INTEGER DEFAULT 0`, "add proxies.canvas_x"},
			{`ALTER TABLE proxies ADD COLUMN canvas_y INTEGER DEFAULT 0`, "add proxies.canvas_y"},
			{`ALTER TABLE proxies ADD COLUMN ram_mb INTEGER DEFAULT 512`, "add proxies.ram_mb"},
			{`ALTER TABLE proxies ADD COLUMN plugin_mc_version TEXT DEFAULT '1.21.11'`, "add proxies.plugin_mc_version"},
			{`ALTER TABLE proxies ADD COLUMN jar_version TEXT`, "add proxies.jar_version"},
			{`ALTER TABLE proxies ADD COLUMN jar_build INTEGER DEFAULT 0`, "add proxies.jar_build"},
			{`ALTER TABLE proxies ADD COLUMN start_on_boot INTEGER DEFAULT 0`, "add proxies.start_on_boot"},
			{`ALTER TABLE proxies ADD COLUMN scheduled_start TEXT`, "add proxies.scheduled_start"},
			{`ALTER TABLE proxies ADD COLUMN scheduled_stop TEXT`, "add proxies.scheduled_stop"},
			{`ALTER TABLE proxies ADD COLUMN jvm_flags TEXT`, "add proxies.jvm_flags"},
			{`ALTER TABLE proxies ADD COLUMN motd_line1 TEXT`, "add proxies.motd_line1"},
			{`ALTER TABLE proxies ADD COLUMN motd_line2 TEXT`, "add proxies.motd_line2"},
			{`ALTER TABLE servers ADD COLUMN minimotd_line1 TEXT`, "add servers.minimotd_line1"},
			{`ALTER TABLE servers ADD COLUMN minimotd_line2 TEXT`, "add servers.minimotd_line2"},
			{`ALTER TABLE servers ADD COLUMN jar_build INTEGER DEFAULT 0`, "add servers.jar_build"},
			{`ALTER TABLE servers ADD COLUMN jar_hash TEXT`, "add servers.jar_hash"},
			{`ALTER TABLE servers ADD COLUMN start_on_boot INTEGER DEFAULT 0`, "add servers.start_on_boot"},
			{`ALTER TABLE servers ADD COLUMN jvm_flags TEXT`, "add servers.jvm_flags"},
			{`ALTER TABLE installed_plugins ADD COLUMN loader_type TEXT DEFAULT ''`, "add installed_plugins.loader_type"},
		},
		4: {
			{`ALTER TABLE servers ADD COLUMN java_override TEXT`, "add servers.java_override"},
		},
		5: {
			{`ALTER TABLE servers ADD COLUMN sanitized_name TEXT`, "add servers.sanitized_name"},
			{`ALTER TABLE proxies ADD COLUMN sanitized_name TEXT`, "add proxies.sanitized_name"},
		},
	}

	var alterVersions []int
	for v := range alterMigrations {
		alterVersions = append(alterVersions, v)
	}
	sort.Ints(alterVersions)

	for _, version := range alterVersions {
		if currentVersion >= version {
			continue
		}

		var applied bool
		for _, alter := range alterMigrations[version] {
			if _, err := db.Exec(alter.sql); err != nil {
				log.Printf("Migration v%d [%s]: %v (skipping)", version, alter.desc, err)
			} else {
				applied = true
			}
		}

		if applied {
			setSchemaVersion(db, version)
			currentVersion = version
		}
	}

	recreateMigrations := map[int]struct {
		name      string
		oldTable  string
		newTable  string
		createDDL string
		copyData  string
	}{
		2: {
			name:     "add_unique_constraint_to_players_uuid",
			oldTable: "players",
			newTable: "players_new",
			createDDL: `CREATE TABLE players_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				uuid TEXT NOT NULL UNIQUE,
				server_id INTEGER NOT NULL,
				joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
			)`,
			copyData: `INSERT INTO players_new (id, name, uuid, server_id, joined_at) 
				SELECT id, name, uuid, server_id, joined_at FROM players`,
		},
		3: {
			name:     "update_backups_table_schema",
			oldTable: "backups",
			newTable: "backups_new",
			createDDL: `CREATE TABLE backups_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				size_bytes INTEGER NOT NULL,
				archive_path TEXT NOT NULL,
				status TEXT DEFAULT 'complete'
			)`,
			copyData: `INSERT INTO backups_new (id, created_at, size_bytes, archive_path, status) 
				SELECT id, created_at, size_bytes, '', 'complete' FROM backups`,
		},
	}

	var recreateVersions []int
	for v := range recreateMigrations {
		recreateVersions = append(recreateVersions, v)
	}
	sort.Ints(recreateVersions)

	for _, version := range recreateVersions {
		if currentVersion >= version {
			continue
		}

		rm := recreateMigrations[version]
		var shouldMigrate bool

		if rm.name == "add_unique_constraint_to_players_uuid" {
			var hasUnique bool
			db.QueryRow("SELECT COUNT(*) > 0 FROM sqlite_master WHERE type='table' AND name=? AND sql LIKE '%uuid TEXT NOT NULL UNIQUE%'", rm.oldTable).Scan(&hasUnique)
			if !hasUnique {
				shouldMigrate = true
			}
		} else if rm.name == "update_backups_table_schema" {
			var hasStatusColumn int
			db.QueryRow("SELECT COUNT(*) FROM pragma_table_info(?) WHERE name='status'", rm.oldTable).Scan(&hasStatusColumn)
			if hasStatusColumn == 0 {
				shouldMigrate = true
			}
		}

		if !shouldMigrate {
			setSchemaVersion(db, version)
			continue
		}

		if _, err := db.Exec(rm.createDDL); err != nil {
			log.Printf("Migration v%d [%s]: failed to create new table: %v", version, rm.name, err)
			continue
		}

		if _, err := db.Exec(rm.copyData); err != nil {
			log.Printf("Migration v%d [%s]: failed to copy data: %v", version, rm.name, err)
			db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", rm.newTable))
			continue
		}

		if _, err := db.Exec(fmt.Sprintf("DROP TABLE %s", rm.oldTable)); err != nil {
			log.Printf("Migration v%d [%s]: failed to drop old table: %v", version, rm.name, err)
			continue
		}

		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s RENAME TO %s", rm.newTable, rm.oldTable)); err != nil {
			log.Printf("Migration v%d [%s]: failed to rename table: %v", version, rm.name, err)
			continue
		}

		if rm.oldTable == "players" {
			db.Exec(fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_players_server ON %s(server_id)", rm.oldTable))
		}

		setSchemaVersion(db, version)
	}

	db.Exec(`UPDATE installed_plugins SET loader_type = '' WHERE loader_type IS NULL`)

	migrateSanitizedNames(db)

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_proxies_status ON proxies(status)`,
		`CREATE INDEX IF NOT EXISTS idx_servers_status ON servers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_servers_proxy ON servers(proxy_id)`,
		`CREATE INDEX IF NOT EXISTS idx_players_server ON players(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_players_uuid ON players(uuid)`,
		`CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status)`,
		`CREATE INDEX IF NOT EXISTS idx_crash_logs_server ON crash_logs(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_command_history_server ON command_history(server_id)`,
		`CREATE INDEX IF NOT EXISTS idx_installed_plugins_target ON installed_plugins(target_type, target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_installed_plugins_project ON installed_plugins(project_id)`,
	}

	for _, index := range indexes {
		if _, err := db.Exec(index); err != nil {
			return fmt.Errorf("index creation failed: %w", err)
		}
	}

	return nil
}

func getSchemaVersion(db *sql.DB) int {
	var version int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_version").Scan(&version)
	if err != nil {
		return 0
	}
	return version
}

func setSchemaVersion(db *sql.DB, version int) {
	db.Exec("INSERT OR REPLACE INTO schema_version (version) VALUES (?)", version)
	log.Printf("Schema migration v%d applied", version)
}

func migrateSanitizedNames(db *sql.DB) {
	var count int
	db.QueryRow("SELECT COUNT(*) FROM servers WHERE sanitized_name IS NULL").Scan(&count)
	if count == 0 {
		db.QueryRow("SELECT COUNT(*) FROM proxies WHERE sanitized_name IS NULL").Scan(&count)
	}
	if count == 0 {
		return
	}

	dataDir := resolveServersDir(db)
	if dataDir != "" {
		log.Printf("Migrating sanitized names and renaming directories under %s", dataDir)
	}

	type nameEntry struct {
		id   int64
		name string
	}

	var serverEntries []nameEntry
	rows, err := db.Query("SELECT id, name FROM servers WHERE sanitized_name IS NULL")
	if err == nil {
		for rows.Next() {
			var e nameEntry
			if err := rows.Scan(&e.id, &e.name); err != nil {
				continue
			}
			serverEntries = append(serverEntries, e)
		}
		rows.Close()
	}

	for _, e := range serverEntries {
		sanitized := util.SanitizeName(e.name)
		db.Exec("UPDATE servers SET sanitized_name = ? WHERE id = ?", sanitized, e.id)

		if dataDir != "" && e.name != sanitized {
			oldPath := filepath.Join(dataDir, e.name)
			newPath := filepath.Join(dataDir, sanitized)
			if _, err := os.Stat(oldPath); err == nil {
				if _, err := os.Stat(newPath); err != nil {
					if err := os.Rename(oldPath, newPath); err != nil {
						log.Printf("Failed to rename server dir %s -> %s: %v", oldPath, newPath, err)
					} else {
						log.Printf("Renamed server dir: %s -> %s", e.name, sanitized)
					}
				}
			}
		}
	}

	var proxyEntries []nameEntry
	proxyRows, err := db.Query("SELECT id, name FROM proxies WHERE sanitized_name IS NULL")
	if err == nil {
		for proxyRows.Next() {
			var e nameEntry
			if err := proxyRows.Scan(&e.id, &e.name); err != nil {
				continue
			}
			proxyEntries = append(proxyEntries, e)
		}
		proxyRows.Close()
	}

	for _, e := range proxyEntries {
		sanitized := util.SanitizeName(e.name)
		db.Exec("UPDATE proxies SET sanitized_name = ? WHERE id = ?", sanitized, e.id)

		if dataDir != "" && e.name != sanitized {
			oldPath := filepath.Join(dataDir, e.name)
			newPath := filepath.Join(dataDir, sanitized)
			if _, err := os.Stat(oldPath); err == nil {
				if _, err := os.Stat(newPath); err != nil {
					if err := os.Rename(oldPath, newPath); err != nil {
						log.Printf("Failed to rename proxy dir %s -> %s: %v", oldPath, newPath, err)
					} else {
						log.Printf("Renamed proxy dir: %s -> %s", e.name, sanitized)
					}
				}
			}
		}
	}

	migrateProxyConfigs(db, dataDir)

	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_servers_sanitized_name ON servers(sanitized_name)")
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_proxies_sanitized_name ON proxies(sanitized_name)")

	log.Println("Sanitized name migration complete")
}

func resolveServersDir(db *sql.DB) string {
	val := os.Getenv("DEMIMINE_SERVERS_DIR")
	if val != "" {
		return val
	}

	for _, candidate := range []string{"/servers", "/data/servers", "./servers"} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}

func migrateProxyConfigs(db *sql.DB, dataDir string) {
	if dataDir == "" {
		return
	}

	type proxyEntry struct {
		id        int64
		name      string
		sanitized string
	}

	rows, err := db.Query(`
		SELECT p.id, p.name, p.sanitized_name
		FROM proxies p
		WHERE p.sanitized_name IS NOT NULL
	`)
	if err != nil {
		return
	}

	var entries []proxyEntry
	for rows.Next() {
		var e proxyEntry
		if err := rows.Scan(&e.id, &e.name, &e.sanitized); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	rows.Close()

	for _, e := range entries {
		proxyPath := filepath.Join(dataDir, e.sanitized)
		configPath := filepath.Join(proxyPath, "plugins", "demiauth", "config.toml")
		if content, err := os.ReadFile(configPath); err == nil {
			oldEntry := fmt.Sprintf(`loginServer = "%s"`, e.name)
			newEntry := fmt.Sprintf(`loginServer = "%s"`, e.sanitized)
			updated := strings.ReplaceAll(string(content), oldEntry, newEntry)

			re := regexp.MustCompile(`loginServer\s*=\s*"[^"]*"`)
			updated = re.ReplaceAllString(updated, newEntry)

			if updated != string(content) {
				os.WriteFile(configPath, []byte(updated), 0644)
				log.Printf("Updated DemiAuth config for proxy %s", e.name)
			}
		}

		velocityPath := filepath.Join(proxyPath, "velocity.toml")
		migrateVelocityConfig(db, velocityPath, e.id)
	}
}

func migrateVelocityConfig(db *sql.DB, velocityPath string, proxyID int64) {
	content, err := os.ReadFile(velocityPath)
	if err != nil {
		return
	}

	serverRows, err := db.Query(`
		SELECT s.name, s.sanitized_name
		FROM servers s
		WHERE s.proxy_id = ? AND s.sanitized_name IS NOT NULL
	`, proxyID)
	if err != nil {
		return
	}

	nameMap := make(map[string]string)
	for serverRows.Next() {
		var displayName, sanitized string
		if err := serverRows.Scan(&displayName, &sanitized); err != nil {
			continue
		}
		if displayName != sanitized {
			nameMap[displayName] = sanitized
		}
	}
	serverRows.Close()

	if len(nameMap) == 0 {
		return
	}

	updated := string(content)
	for oldName, newName := range nameMap {
		updated = strings.ReplaceAll(updated, oldName+" = ", newName+" = ")
		updated = strings.ReplaceAll(updated, fmt.Sprintf(`"%s"`, oldName), fmt.Sprintf(`"%s"`, newName))
	}

	if updated != string(content) {
		os.WriteFile(velocityPath, []byte(updated), 0644)
		log.Printf("Updated velocity.toml server names for proxy %d", proxyID)
	}
}
