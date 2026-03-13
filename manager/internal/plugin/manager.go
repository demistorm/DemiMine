package plugin

import (
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/demimine/manager/internal/modrinth"
)

type InstalledPlugin struct {
	ID            int64             `json:"id"`
	TargetType    string            `json:"target_type"`
	TargetID      int64             `json:"target_id"`
	ProjectID     string            `json:"project_id"`
	ProjectSlug   string            `json:"project_slug"`
	ProjectName   string            `json:"project_name"`
	VersionID     string            `json:"version_id"`
	VersionNumber string            `json:"version_number"`
	Filename      string            `json:"filename"`
	FileHash      string            `json:"file_hash"`
	InstalledAt   time.Time         `json:"installed_at"`
	Dependencies  []InstalledPlugin `json:"dependencies,omitempty"`
}

type InstallOptions struct {
	TargetType  string
	TargetID    int64
	ProjectID   string
	VersionID   string
	GameVersion string
	Loaders     []string
	PluginsDir  string
}

type Manager struct {
	db         *sql.DB
	modrinth   *modrinth.Client
	serversDir string
	httpClient *http.Client
}

func NewManager(db *sql.DB, serversDir string) *Manager {
	return &Manager{
		db:         db,
		modrinth:   modrinth.NewClient(),
		serversDir: serversDir,
		httpClient: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (m *Manager) GetInstalled(targetType string, targetID int64) ([]InstalledPlugin, error) {
	rows, err := m.db.Query(`
		SELECT id, target_type, target_id, project_id, project_slug, project_name,
		       version_id, version_number, filename, file_hash, installed_at
		FROM installed_plugins
		WHERE target_type = ? AND target_id = ?
		ORDER BY project_name
	`, targetType, targetID)
	if err != nil {
		return nil, fmt.Errorf("failed to query installed plugins: %w", err)
	}
	defer rows.Close()

	var plugins []InstalledPlugin
	for rows.Next() {
		var p InstalledPlugin
		err := rows.Scan(
			&p.ID, &p.TargetType, &p.TargetID, &p.ProjectID, &p.ProjectSlug, &p.ProjectName,
			&p.VersionID, &p.VersionNumber, &p.Filename, &p.FileHash, &p.InstalledAt,
		)
		if err != nil {
			continue
		}
		plugins = append(plugins, p)
	}

	return plugins, nil
}

func (m *Manager) GetInstalledByProject(targetType string, targetID int64, projectID string) (*InstalledPlugin, error) {
	var p InstalledPlugin
	err := m.db.QueryRow(`
		SELECT id, target_type, target_id, project_id, project_slug, project_name,
		       version_id, version_number, filename, file_hash, installed_at
		FROM installed_plugins
		WHERE target_type = ? AND target_id = ? AND project_id = ?
	`, targetType, targetID, projectID).Scan(
		&p.ID, &p.TargetType, &p.TargetID, &p.ProjectID, &p.ProjectSlug, &p.ProjectName,
		&p.VersionID, &p.VersionNumber, &p.Filename, &p.FileHash, &p.InstalledAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query installed plugin: %w", err)
	}
	return &p, nil
}

func (m *Manager) Install(opts InstallOptions) (*InstalledPlugin, error) {
	existing, err := m.GetInstalledByProject(opts.TargetType, opts.TargetID, opts.ProjectID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("plugin already installed")
	}

	var version *modrinth.Version
	if opts.VersionID != "" {
		version, err = m.modrinth.GetVersion(opts.VersionID)
	} else {
		var versions []modrinth.Version
		versions, err = m.modrinth.GetProjectVersions(opts.ProjectID, []string{opts.GameVersion}, opts.Loaders)
		if err == nil {
			version = modrinth.FindBestVersion(versions, opts.GameVersion, opts.Loaders)
			if version == nil {
				err = fmt.Errorf("no compatible version found")
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get version: %w", err)
	}

	file := version.GetPrimaryFile()
	if file == nil {
		return nil, fmt.Errorf("no file found in version")
	}

	targetPath := filepath.Join(m.serversDir, opts.PluginsDir, "plugins")
	if err := os.MkdirAll(targetPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create plugins directory: %w", err)
	}

	destPath := filepath.Join(targetPath, file.Filename)
	if err := m.downloadFile(file.URL, destPath); err != nil {
		return nil, fmt.Errorf("failed to download plugin: %w", err)
	}

	project, err := m.modrinth.GetProject(opts.ProjectID)
	if err != nil {
		project = &modrinth.Project{ID: opts.ProjectID, Title: opts.ProjectID, Slug: opts.ProjectID}
	}

	result, err := m.db.Exec(`
		INSERT INTO installed_plugins (target_type, target_id, project_id, project_slug, project_name,
		                               version_id, version_number, filename, file_hash)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, opts.TargetType, opts.TargetID, opts.ProjectID, project.Slug, project.Title,
		version.ID, version.VersionNumber, file.Filename, file.Hashes.SHA1)
	if err != nil {
		os.Remove(destPath)
		return nil, fmt.Errorf("failed to record installation: %w", err)
	}

	id, _ := result.LastInsertId()

	return &InstalledPlugin{
		ID:            id,
		TargetType:    opts.TargetType,
		TargetID:      opts.TargetID,
		ProjectID:     opts.ProjectID,
		ProjectSlug:   project.Slug,
		ProjectName:   project.Title,
		VersionID:     version.ID,
		VersionNumber: version.VersionNumber,
		Filename:      file.Filename,
		FileHash:      file.Hashes.SHA1,
		InstalledAt:   time.Now(),
	}, nil
}

func (m *Manager) downloadFile(url, dest string) error {
	resp, err := m.httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

func (m *Manager) Uninstall(targetType string, targetID int64, projectID string) error {
	var filename, pluginsDir string
	var targetName string

	if targetType == "server" {
		err := m.db.QueryRow("SELECT name FROM servers WHERE id = ?", targetID).Scan(&targetName)
		if err != nil {
			return fmt.Errorf("server not found")
		}
		pluginsDir = targetName
	} else if targetType == "proxy" {
		err := m.db.QueryRow("SELECT name FROM proxies WHERE id = ?", targetID).Scan(&targetName)
		if err != nil {
			return fmt.Errorf("proxy not found")
		}
		pluginsDir = targetName
	}

	err := m.db.QueryRow(`
		SELECT filename FROM installed_plugins
		WHERE target_type = ? AND target_id = ? AND project_id = ?
	`, targetType, targetID, projectID).Scan(&filename)
	if err == sql.ErrNoRows {
		return fmt.Errorf("plugin not found")
	}
	if err != nil {
		return fmt.Errorf("failed to get plugin info: %w", err)
	}

	pluginPath := filepath.Join(m.serversDir, pluginsDir, "plugins", filename)
	if err := os.Remove(pluginPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete plugin file: %w", err)
	}

	_, err = m.db.Exec(`
		DELETE FROM installed_plugins
		WHERE target_type = ? AND target_id = ? AND project_id = ?
	`, targetType, targetID, projectID)
	if err != nil {
		return fmt.Errorf("failed to remove plugin record: %w", err)
	}

	return nil
}

type UpdateResult struct {
	Plugin         InstalledPlugin `json:"plugin"`
	LatestVersion  string          `json:"latest_version"`
	CurrentVersion string          `json:"current_version"`
	HasUpdate      bool            `json:"has_update"`
}

func (m *Manager) CheckForUpdates(targetType string, targetID int64, gameVersion string, loaders []string) ([]UpdateResult, error) {
	plugins, err := m.GetInstalled(targetType, targetID)
	if err != nil {
		return nil, err
	}

	var results []UpdateResult
	for _, p := range plugins {
		versions, err := m.modrinth.GetProjectVersions(p.ProjectID, []string{gameVersion}, loaders)
		if err != nil {
			continue
		}

		latest := modrinth.FindBestVersion(versions, gameVersion, loaders)
		if latest == nil {
			continue
		}

		results = append(results, UpdateResult{
			Plugin:         p,
			LatestVersion:  latest.VersionNumber,
			CurrentVersion: p.VersionNumber,
			HasUpdate:      latest.ID != p.VersionID,
		})
	}

	return results, nil
}

func (m *Manager) Update(targetType string, targetID int64, projectID string, gameVersion string, loaders []string) (*InstalledPlugin, error) {
	existing, err := m.GetInstalledByProject(targetType, targetID, projectID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("plugin not installed")
	}

	versions, err := m.modrinth.GetProjectVersions(projectID, []string{gameVersion}, loaders)
	if err != nil {
		return nil, fmt.Errorf("failed to get versions: %w", err)
	}

	latest := modrinth.FindBestVersion(versions, gameVersion, loaders)
	if latest == nil {
		return nil, fmt.Errorf("no compatible version found")
	}

	if latest.ID == existing.VersionID {
		return existing, nil
	}

	var targetName string
	if targetType == "server" {
		err := m.db.QueryRow("SELECT name FROM servers WHERE id = ?", targetID).Scan(&targetName)
		if err != nil {
			return nil, fmt.Errorf("server not found")
		}
	} else {
		err := m.db.QueryRow("SELECT name FROM proxies WHERE id = ?", targetID).Scan(&targetName)
		if err != nil {
			return nil, fmt.Errorf("proxy not found")
		}
	}

	file := latest.GetPrimaryFile()
	if file == nil {
		return nil, fmt.Errorf("no file found in version")
	}

	targetPath := filepath.Join(m.serversDir, targetName, "plugins")
	oldPath := filepath.Join(targetPath, existing.Filename)
	os.Remove(oldPath)

	destPath := filepath.Join(targetPath, file.Filename)
	if err := m.downloadFile(file.URL, destPath); err != nil {
		return nil, fmt.Errorf("failed to download plugin: %w", err)
	}

	_, err = m.db.Exec(`
		UPDATE installed_plugins
		SET version_id = ?, version_number = ?, filename = ?, file_hash = ?, installed_at = CURRENT_TIMESTAMP
		WHERE target_type = ? AND target_id = ? AND project_id = ?
	`, latest.ID, latest.VersionNumber, file.Filename, file.Hashes.SHA1, targetType, targetID, projectID)
	if err != nil {
		os.Remove(destPath)
		return nil, fmt.Errorf("failed to update plugin record: %w", err)
	}

	existing.VersionID = latest.ID
	existing.VersionNumber = latest.VersionNumber
	existing.Filename = file.Filename
	existing.FileHash = file.Hashes.SHA1
	existing.InstalledAt = time.Now()

	return existing, nil
}

func (m *Manager) GetDependencies(projectID string, gameVersion string, loaders []string) ([]modrinth.Dependency, error) {
	versions, err := m.modrinth.GetProjectVersions(projectID, []string{gameVersion}, loaders)
	if err != nil {
		return nil, err
	}

	latest := modrinth.FindBestVersion(versions, gameVersion, loaders)
	if latest == nil {
		return nil, fmt.Errorf("no compatible version found")
	}

	return latest.Dependencies, nil
}
