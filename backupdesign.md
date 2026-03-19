# Phase 5: Backup System Design

## Overview

Full network snapshots with tar+zstd compression, scheduled every 3 days at configurable time, keeping the last 2 backups. Supports host export for offsite storage, "start on manager boot" flag per server/proxy, and restore from uploaded file.

**Key Behavior:**
- Manager on = Network on (servers/proxies with `start_on_boot` flag auto-start)
- Manager off = Network off (all servers/proxies gracefully stop when manager stops)

## Features

- **Full network snapshots** - All servers, proxies, database, and Java runtimes in one archive
- **Scheduled backups** - Runs every 3 days at configurable time (only if running at that time)
- **Retention policy** - Keeps only the 2 most recent backups (based on date in filename)
- **Host export** - Optional export to configured host path for offsite storage
- **Manual backup** - Trigger backup on-demand from settings page
- **Restore** - Upload and restore from backup file, triggers manager restart
- **Start on boot** - Per server/proxy flag to auto-start when manager boots (e.g., proxies, queue servers, login servers)
- **Graceful shutdown** - All servers/proxies stop gracefully when manager shuts down
- **Auto-start behavior** - Manager on = network on (for flagged servers/proxies), Manager off = network off
- **Progress indicator** - Banner shows "Backup in progress" in top bar

## Tech Stack

- **Compression**: `tar` + `zstd` (fast compression, good compatibility)
- **Database**: SQLite with new `backups` table
- **Scheduler**: Go `time.Ticker` with daily checks
- **Archive format**: `demimine-backup-YYYY-MM-DD.tar.zst`

## Database Schema

### New Table: `backups`

```sql
CREATE TABLE backups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    size_bytes INTEGER NOT NULL,
    archive_path TEXT NOT NULL,
    status TEXT DEFAULT 'complete'
);
```

### Modified Table: `servers`

```sql
ALTER TABLE servers ADD COLUMN start_on_boot INTEGER DEFAULT 0;
-- Remove: backup_interval_days column
```

### Modified Table: `proxies`

```sql
ALTER TABLE proxies ADD COLUMN start_on_boot INTEGER DEFAULT 0;
```

### Settings Keys

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `backup_time` | string | `"03:00"` | When to run backup (24h format) |
| `backup_host_path` | string | `""` | Host path to copy backup to (empty = disabled) |
| `last_backup_date` | string | `""` | Date of last backup (YYYY-MM-DD) - used to check if 3+ days since last backup |

## Data Models

### Backup (models.go)

```go
type Backup struct {
    ID         int64     `json:"id"`
    CreatedAt  time.Time `json:"created_at"`
    SizeBytes  int64     `json:"size_bytes"`
    ArchivePath string   `json:"archive_path"`
    Status     string    `json:"status"`
}
```

### Server (models.go - updated)

```go
type Server struct {
    // ... existing fields ...
    StartOnBoot int    `json:"start_on_boot"`
    // Remove: BackupIntervalDays
}
```

### Proxy (models.go - updated)

```go
type Proxy struct {
    // ... existing fields ...
    StartOnBoot int `json:"start_on_boot"`
}
```

## Backend Architecture

### Package: `internal/backup/`

#### `backup.go` - Core backup logic

```go
type Manager struct {
    db           *sql.DB
    dockerClient *docker.Client
    serverHandler *api.ServersHandler
    proxyHandler  *api.ProxiesHandler
    dataDir       string  // /data
    backupDir     string  // /data/backups
}

func NewManager(...) *Manager

// CreateSnapshot - runs full backup workflow
func (m *Manager) CreateSnapshot(ctx context.Context) (*Backup, error)

// RestoreSnapshot - restores from uploaded tar.zst
func (m *Manager) RestoreSnapshot(ctx context.Context, archivePath string) error

// CleanupOldBackups - keeps only 2 newest by date in filename
func (m *Manager) CleanupOldBackups() error

// ExportToHost - copies to configured host path via Docker cp
func (m *Manager) ExportToHost(archivePath, hostPath string) error

// ShouldRunBackup - checks if 3+ days since last backup
func (m *Manager) ShouldRunBackup() bool
```

**CreateSnapshot Flow:**

```
1. Create backup record in DB (status: pending)
2. Get all servers/proxies with start_on_boot=1 (to restart later)
3. Stop ALL servers gracefully (send "stop" via console, 30s timeout)
4. Stop ALL proxies gracefully (30s timeout)
5. Create archive:
   tar --zstd -cf /data/backups/demimine-backup-YYYY-MM-DD.tar.zst \
       -C /data servers proxies data/demimine.db java
6. Update backup record (status: complete, size_bytes)
7. Call CleanupOldBackups() - keeps last 2
8. Start ONLY servers/proxies with start_on_boot=1
9. Update last_backup_date setting
10. Export to host path if configured
```

**RestoreSnapshot Flow:**

```
1. Validate archive exists and is readable
2. Stop all servers and proxies (should already be stopped from graceful shutdown)
3. Extract tar.zst to temp directory
4. Move current servers/, proxies/, data/demimine.db, java/ aside
5. Move extracted files into place
6. Trigger manager container restart
7. On restart, servers/proxies with start_on_boot=1 auto-start
```

**Manager Startup Behavior:**

```
On manager container start:
1. Load all servers and proxies from database
2. For each server/proxy with start_on_boot=1:
   - Start the server/proxy
   - Update status to 'running'
```

**Manager Shutdown Behavior:**

```
On manager container stop (SIGTERM):
1. Catch shutdown signal
2. Stop all servers gracefully (send "stop", 30s timeout)
3. Stop all proxies gracefully (30s timeout)
4. Exit cleanly
```

**CleanupOldBackups Logic:**

```
1. List all backup files in /data/backups/
2. Sort by date in filename (YYYY-MM-DD pattern)
3. Delete files beyond first 2
4. Delete corresponding DB records
```

#### `scheduler.go` - Daily backup trigger

```go
type Scheduler struct {
    manager *Manager
    ticker  *time.Ticker
}

func (s *Scheduler) Start() {
    s.ticker = time.NewTicker(1 * time.Hour)
    go func() {
        for range s.ticker.C {
            s.checkAndRun()
        }
    }()
}

func (s *Scheduler) Stop() {
    s.ticker.Stop()
}

func (s *Scheduler) checkAndRun() {
    settings := loadSettings()
    currentHour := time.Now().Format("15:04")
    
    // Only run at configured time
    if currentHour == settings.backup_time && s.manager.ShouldRunBackup() {
        s.manager.CreateSnapshot(context.Background())
    }
}
```

### Manager Shutdown Handler (main.go)

```go
func gracefulShutdown(serverHandler *api.ServersHandler, proxyHandler *api.ProxiesHandler) {
    sigchan := make(chan os.Signal, 1)
    signal.Notify(sigchan, os.Interrupt, syscall.SIGTERM)
    
    <-sigchan
    
    log.Println("Shutting down manager...")
    
    // Stop all servers gracefully
    servers, _ := serverHandler.ListAll()
    for _, server := range servers {
        if server.Status == "running" {
            log.Printf("Stopping server: %s", server.Name)
            serverHandler.StopByID(server.ID)
        }
    }
    
    // Stop all proxies gracefully
    proxies, _ := proxyHandler.ListAll()
    for _, proxy := range proxies {
        if proxy.Status == "running" {
            log.Printf("Stopping proxy: %s", proxy.Name)
            proxyHandler.StopByID(proxy.ID)
        }
    }
    
    os.Exit(0)
}
```

### Manager Startup (main.go)

```go
func startFlaggedServers(serverHandler *api.ServersHandler, proxyHandler *api.ProxiesHandler) {
    // Start servers with start_on_boot=1
    servers, _ := serverHandler.ListAll()
    for _, server := range servers {
        if server.StartOnBoot == 1 {
            log.Printf("Auto-starting server: %s", server.Name)
            serverHandler.StartByID(server.ID)
        }
    }
    
    // Start proxies with start_on_boot=1
    proxies, _ := proxyHandler.ListAll()
    for _, proxy := range proxies {
        if proxy.StartOnBoot == 1 {
            log.Printf("Auto-starting proxy: %s", proxy.Name)
            proxyHandler.StartByID(proxy.ID)
        }
    }
}
```

### API Handler: `backups.go`

```go
type BackupsHandler struct {
    db      *sql.DB
    manager *backup.Manager
}

// GET /api/backups - list all backups
func (h *BackupsHandler) List(w http.ResponseWriter, r *http.Request)

// POST /api/backups - trigger manual backup
func (h *BackupsHandler) Create(w http.ResponseWriter, r *http.Request)

// GET /api/backups/:id/download - download backup file
func (h *BackupsHandler) Download(w http.ResponseWriter, r *http.Request)

// POST /api/backups/restore - restore from uploaded file
func (h *BackupsHandler) Restore(w http.ResponseWriter, r *http.Request)

// DELETE /api/backups/:id - delete backup
func (h *BackupsHandler) Delete(w http.ResponseWriter, r *http.Request)
```

### Settings Handler Updates

Add support in `settings.go`:

```go
type SettingsRequest struct {
    ProxyMCVersion    string `json:"proxy_mc_version"`
    BackupTime        string `json:"backup_time"`
    BackupHostPath    string `json:"backup_host_path"`
}
```

## Frontend Architecture

### Settings Page: `routes/settings/+page.svelte`

Add backup section after API Keys:

```
┌─────────────────────────────────────────────────┐
│ Backups                                         │
│                                                 │
│ Backup time        [  03:00  ] (24h format)     │
│ Host export path   [/mnt/backups/demimine]      │
│                                                 │
│ [ Backup Now ]                                  │
│                                                 │
│ Recent Backups                                  │
│ ┌─────────────────────────────────────────────┐ │
│ │ demimine-backup-2026-03-19.tar.zst          │ │
│ │ 2.4 GB • Mar 19, 2026  [⬇] [🗑]             │ │
│ ├─────────────────────────────────────────────┤ │
│ │ demimine-backup-2026-03-16.tar.zst          │ │
│ │ 2.3 GB • Mar 16, 2026  [⬇] [🗑]             │ │
│ └─────────────────────────────────────────────┘ │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│ Restore from Backup                             │
│                                                 │
│ [ Choose File ]  demimine-backup.tar.zst        │
│                                                 │
│ ⚠️ This will replace all servers, proxies,      │
│    and settings. Cannot be undone.              │
│                                                 │
│ [ Restore Backup ]  (disabled until file chosen)│
└─────────────────────────────────────────────────┘
```

### Server Settings: `components/Settings.svelte`

Update "Automation" section:

```svelte
<div class="section">
    <h3>Automation</h3>
    
    <label class="field">
        <span>Auto-shutdown (minutes)</span>
        <input type="number" bind:value={autoShutdown} min="0" />
    </label>
    
    <label class="field checkbox">
        <input type="checkbox" bind:checked={startOnBoot} />
        <span>Start on manager boot (auto-start when manager boots)</span>
    </label>
</div>
```

**Remove:** Backup interval days field

### Proxy Settings: `components/ProxySettings.svelte`

Add "Startup" section:

```svelte
<div class="section">
    <h3>Startup</h3>
    
    <label class="field checkbox">
        <input type="checkbox" bind:checked={startOnBoot} />
        <span>Start on manager boot (auto-start when manager boots)</span>
    </label>
</div>
```

### Backup In Progress Banner

Add to layout or top bar component:

```svelte
{#if $backupStatus === 'in_progress'}
    <div class="backup-banner">
        ⏳ Backup in progress - servers are temporarily stopped
    </div>
{/if}

<style>
    .backup-banner {
        background: #fbbf24;
        color: #1f2937;
        padding: 0.75rem;
        text-align: center;
        font-weight: 500;
    }
</style>
```

**WebSocket message type:**
```typescript
{ type: 'backup_status', status: 'in_progress' | 'complete' | 'failed' }
```

### API Client: `lib/api.ts`

```typescript
export const backupsApi = {
    list: async () => {
        return await api.get('/api/backups');
    },
    
    create: async () => {
        return await api.post('/api/backups');
    },
    
    download: async (id: number) => {
        const res = await fetch(`/api/backups/${id}/download`);
        const blob = await res.blob();
        const url = window.URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `demimine-backup-${id}.tar.zst`;
        a.click();
    },
    
    restore: async (file: File) => {
        const form = new FormData();
        form.append('backup', file);
        return await fetch('/api/backups/restore', { method: 'POST', body: form });
    },
    
    delete: async (id: number) => {
        return await api.delete(`/api/backups/${id}`);
    }
};
```

### Settings Store: `lib/stores/settings.ts`

```typescript
interface Settings {
    proxy_mc_version: string;
    backup_time: string;
    backup_host_path: string;
}

export const globalSettings = writable<Settings>({
    proxy_mc_version: '1.21.11',
    backup_time: '03:00',
    backup_host_path: ''
});
```

## Docker Changes

### Dockerfile

Add zstd package for compression:

```dockerfile
RUN apk add --no-cache zstd
```

## File Structure

### Backend

```
manager/
├── internal/
│   ├── backup/
│   │   ├── backup.go        # Core backup/restore logic
│   │   └── scheduler.go     # Daily check and trigger
│   ├── api/
│   │   └── handlers/
│   │       ├── backups.go   # New backup API handler
│   │       ├── settings.go  # Updated for backup settings
│   │       ├── servers.go   # Updated for start_on_boot
│   │       └── proxies.go   # Updated for start_on_boot
│   ├── db/
│   │   └── db.go            # Schema changes
│   ├── models/
│   │   └── models.go        # Backup struct, Server/Proxy updates
│   └── docker/
│       └── client.go        # Ensure export methods available
└── cmd/server/
    └── main.go              # Initialize backup manager & scheduler, add shutdown/startup handlers
```

### Frontend

```
webui/src/
├── routes/
│   └── settings/
│       └── +page.svelte     # Backup settings UI
├── lib/
│   ├── components/
│   │   ├── Settings.svelte          # Add start_on_boot checkbox
│   │   ├── ProxySettings.svelte     # Add start_on_boot checkbox
│   │   └── TopBar.svelte            # Add backup banner
│   ├── api.ts             # Add backupsApi
│   └── stores/
│       └── settings.ts     # Add backup settings
```

## Implementation Order

1. **Database schema** - Add `backups` table, `start_on_boot` columns, remove `backup_interval_days`
2. **Models** - Add `Backup` struct, update `Server`/`Proxy` with `StartOnBoot`
3. **Backup package** - Implement `backup.go` (snapshot, restore, cleanup, export)
4. **Scheduler** - Implement `scheduler.go` (daily check at configured time)
5. **API handlers** - Implement `backups.go`, update `settings.go`, `servers.go`, `proxies.go`
6. **Main.go** - Initialize backup manager & scheduler, wire up routes, add shutdown handler
7. **Dockerfile** - Add zstd package
8. **Frontend settings page** - Backup configuration UI
9. **Frontend server/proxy settings** - Start on boot checkboxes
10. **Frontend backup banner** - In-progress indicator via WebSocket
11. **Build & test** - Full backup/restore cycle, startup/shutdown behavior

## Testing Checklist

- [ ] Manual backup creates archive in /data/backups/
- [ ] Archive contains: servers/, proxies/, data/demimine.db, java/
- [ ] Cleanup keeps only 2 most recent backups
- [ ] Scheduled backup runs at configured time (only if manager running)
- [ ] Backup doesn't run if < 3 days since last backup
- [ ] Servers with start_on_boot=1 start after backup
- [ ] Servers with start_on_boot=0 do NOT start after backup
- [ ] Restore from uploaded file works
- [ ] Restore triggers manager restart
- [ ] Host export copies backup to configured path
- [ ] Backup in progress banner displays
- [ ] Frontend shows correct backup list
- [ ] Download button works
- [ ] Delete button removes file and DB record
- [ ] **Startup**: Servers/proxies with start_on_boot=1 auto-start when manager boots
- [ ] **Startup**: Servers/proxies with start_on_boot=0 do NOT auto-start when manager boots
- [ ] **Shutdown**: All servers/proxies stop gracefully when manager container stops
- [ ] **Shutdown**: No orphaned containers remain after manager shutdown

## Known Limitations

- Backup only runs if manager is running at the scheduled time (no catch-up)
- Restore requires manager restart (short downtime)
- Large networks (10+ servers with big worlds) may take several minutes to backup/restore
- Host export requires Docker socket access and appropriate permissions
- Manager shutdown stops ALL servers/proxies (cannot selectively stop manager only)

## Future Enhancements (Phase 6)

- Backup retention policy configuration (keep N backups)
- Multiple backup schedules (daily, weekly)
- Incremental backups (diff from last backup)
- Backup verification (test extract, validate integrity)
- Backup encryption
- Offsite upload (S3, Google Drive, etc.)
