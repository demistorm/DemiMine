package models

import "time"

type Proxy struct {
	ID               int64     `json:"id"`
	Name             string    `json:"name" binding:"required"`
	HostPort         int       `json:"host_port" binding:"required"`
	RAMMB            int       `json:"ram_mb"`
	ForwardingSecret string    `json:"forwarding_secret"`
	Status           string    `json:"status"`
	CanvasX          int       `json:"canvas_x"`
	CanvasY          int       `json:"canvas_y"`
	MotdLine1        *string   `json:"motd_line1"`
	MotdLine2        *string   `json:"motd_line2"`
	JVMFlags         *string   `json:"jvm_flags"`
	StartOnBoot      int       `json:"start_on_boot"`
	ScheduledStart   *string   `json:"scheduled_start"`
	ScheduledStop    *string   `json:"scheduled_stop"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type Server struct {
	ID                  int64     `json:"id"`
	Name                string    `json:"name" binding:"required"`
	Type                string    `json:"type" binding:"required"`
	Version             string    `json:"version" binding:"required"`
	ProxyID             *int64    `json:"proxy_id"`
	HostPort            *int      `json:"host_port"`
	RAMMB               int       `json:"ram_mb" binding:"required"`
	Domain              *string   `json:"domain"`
	BackupIntervalDays  int       `json:"backup_interval_days"`
	AutoShutdownMinutes int       `json:"auto_shutdown_minutes"`
	ScheduledStart      *string   `json:"scheduled_start"`
	ScheduledStop       *string   `json:"scheduled_stop"`
	Status              string    `json:"status"`
	CanvasX             int       `json:"canvas_x"`
	CanvasY             int       `json:"canvas_y"`
	JVMFlags            *string   `json:"jvm_flags"`
	JavaOverride        *string   `json:"java_override"`
	StartOnBoot         int       `json:"start_on_boot"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

type Player struct {
	ID       int64     `json:"id"`
	Name     string    `json:"name"`
	UUID     string    `json:"uuid"`
	ServerID int64     `json:"server_id"`
	JoinedAt time.Time `json:"joined_at"`
}

type APIKey struct {
	ID         int64      `json:"id"`
	KeyHash    string     `json:"-"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

type Setting struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type JavaVersion struct {
	Version      string    `json:"version"`
	Path         string    `json:"path"`
	DownloadedAt time.Time `json:"downloaded_at"`
}

type Backup struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	SizeBytes   int64     `json:"size_bytes"`
	ArchivePath string    `json:"archive_path"`
	Status      string    `json:"status"`
}

type CrashLog struct {
	ID           int64     `json:"id"`
	ServerID     int64     `json:"server_id"`
	Content      string    `json:"content"`
	CreatedAt    time.Time `json:"created_at"`
	Acknowledged bool      `json:"acknowledged"`
}

type CommandHistory struct {
	ID         int64     `json:"id"`
	ServerID   int64     `json:"server_id"`
	Command    string    `json:"command"`
	ExecutedAt time.Time `json:"executed_at"`
}

type AdminAuth struct {
	ID            int64  `json:"id"`
	Username      string `json:"username"`
	PasswordHash  string `json:"-"`
	SetupComplete bool   `json:"setup_complete"`
}

type ServerResponse struct {
	ID                  int64   `json:"id"`
	Name                string  `json:"name"`
	Type                string  `json:"type"`
	Version             string  `json:"version"`
	ProxyID             *int64  `json:"proxy_id"`
	ProxyName           *string `json:"proxy_name"`
	RAMMB               int     `json:"ram_mb"`
	Domain              *string `json:"domain"`
	BackupIntervalDays  int     `json:"backup_interval_days"`
	AutoShutdownMinutes int     `json:"auto_shutdown_minutes"`
	ScheduledStart      *string `json:"scheduled_start"`
	ScheduledStop       *string `json:"scheduled_stop"`
	HostPort            *int    `json:"host_port"`
	Status              string  `json:"status"`
	PlayerCount         int     `json:"player_count"`
	CanvasX             int     `json:"canvas_x"`
	CanvasY             int     `json:"canvas_y"`
	StartOnBoot         int     `json:"start_on_boot"`
	JVMFlags            *string `json:"jvm_flags"`
	JavaOverride        *string `json:"java_override"`
	IconPath            *string `json:"icon_path"`
	MinimotdLine1       *string `json:"minimotd_line1"`
	MinimotdLine2       *string `json:"minimotd_line2"`
	JarBuild            int     `json:"jar_build"`
	CreatedAt           string  `json:"created_at"`
}

type ProxyResponse struct {
	ID               int64    `json:"id"`
	Name             string   `json:"name"`
	HostPort         int      `json:"host_port"`
	RAMMB            int      `json:"ram_mb"`
	ForwardingSecret string   `json:"forwarding_secret"`
	Status           string   `json:"status"`
	PlayerCount      int      `json:"player_count"`
	ConnectedServers []string `json:"connected_servers"`
	CanvasX          int      `json:"canvas_x"`
	CanvasY          int      `json:"canvas_y"`
	MotdLine1        *string  `json:"motd_line1"`
	MotdLine2        *string  `json:"motd_line2"`
	JVMFlags         *string  `json:"jvm_flags"`
	StartOnBoot      int      `json:"start_on_boot"`
	ScheduledStart   *string  `json:"scheduled_start"`
	ScheduledStop    *string  `json:"scheduled_stop"`
	IconPath         *string  `json:"icon_path"`
	JarVersion       *string  `json:"jar_version"`
	JarBuild         int      `json:"jar_build"`
	CreatedAt        string   `json:"created_at"`
}
