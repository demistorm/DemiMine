package spark

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/demimine/manager/internal/plugin"
)

const (
	sparkProjectSlug = "spark"
	sparkJenkinsAPI  = "https://ci.lucko.me/job/spark/lastSuccessfulBuild/api/json"
)

type JenkinsBuild struct {
	Artifacts []JenkinsArtifact `json:"artifacts"`
}

type JenkinsArtifact struct {
	RelativePath string `json:"relativePath"`
}

type Installer struct {
	pluginManager *plugin.Manager
	httpClient    *http.Client
	serversDir    string
	db            *sql.DB
}

func NewInstaller(pluginManager *plugin.Manager, serversDir string, db *sql.DB) *Installer {
	return &Installer{
		pluginManager: pluginManager,
		serversDir:    serversDir,
		db:            db,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (i *Installer) InstallSparkForServer(serverType, mcVersion, serverName string, serverID int64) error {
	var sanitizedName string
	if err := i.db.QueryRow("SELECT sanitized_name FROM servers WHERE id = ?", serverID).Scan(&sanitizedName); err != nil {
		return fmt.Errorf("failed to get server sanitized name: %w", err)
	}

	switch serverType {
	case "paper", "purpur", "nanolimbo":
		log.Printf("[Spark] Skipping Spark installation for %s server %s (bundled or not applicable)", serverType, sanitizedName)
		return nil
	case "fabric", "neoforge", "forge":
		return i.installSparkViaModrinth(serverType, mcVersion, sanitizedName, serverID)
	default:
		return fmt.Errorf("unsupported server type for Spark installation: %s", serverType)
	}
}

func (i *Installer) installSparkViaModrinth(serverType, mcVersion, serverName string, serverID int64) error {
	loaders := getLoadersForServerType(serverType)
	if loaders == nil {
		return fmt.Errorf("no loaders defined for server type: %s", serverType)
	}

	pluginsDir := filepath.Join(i.serversDir, serverName, "mods")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create mods directory: %w", err)
	}

	log.Printf("[Spark] Installing Spark for %s server %s (MC %s) via Modrinth", serverType, serverName, mcVersion)

	_, err := i.pluginManager.Install(plugin.InstallOptions{
		TargetType:   "server",
		TargetID:     serverID,
		ProjectID:    sparkProjectSlug,
		GameVersion:  mcVersion,
		Loaders:      loaders,
		ServerName:   serverName,
		TargetSubdir: "mods",
	})

	if err != nil {
		return fmt.Errorf("failed to install Spark via Modrinth: %w", err)
	}

	log.Printf("[Spark] Successfully installed Spark for server %s", serverName)
	return nil
}

func (i *Installer) InstallSparkForProxy(proxyName string, proxyID int64) error {
	var sanitizedName string
	if err := i.db.QueryRow("SELECT sanitized_name FROM proxies WHERE id = ?", proxyID).Scan(&sanitizedName); err != nil {
		return fmt.Errorf("failed to get proxy sanitized name: %w", err)
	}

	pluginsDir := filepath.Join(i.serversDir, sanitizedName, "plugins")
	if err := os.MkdirAll(pluginsDir, 0755); err != nil {
		return fmt.Errorf("failed to create plugins directory: %w", err)
	}

	log.Printf("[Spark] Installing Spark for Velocity proxy %s via Jenkins CI", sanitizedName)

	jarPath, err := i.downloadLatestVelocitySpark(pluginsDir)
	if err != nil {
		return fmt.Errorf("failed to download Spark for Velocity: %w", err)
	}

	log.Printf("[Spark] Successfully installed Spark for proxy %s: %s", sanitizedName, filepath.Base(jarPath))
	return nil
}

func (i *Installer) downloadLatestVelocitySpark(destDir string) (string, error) {
	resp, err := i.httpClient.Get(sparkJenkinsAPI)
	if err != nil {
		return "", fmt.Errorf("failed to fetch Jenkins API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Jenkins API returned status %d", resp.StatusCode)
	}

	var build JenkinsBuild
	if err := json.NewDecoder(resp.Body).Decode(&build); err != nil {
		return "", fmt.Errorf("failed to decode Jenkins build: %w", err)
	}

	var velocityArtifact *JenkinsArtifact
	for _, artifact := range build.Artifacts {
		if containsString(artifact.RelativePath, "spark-velocity/build/libs/") {
			velocityArtifact = &artifact
			break
		}
	}

	if velocityArtifact == nil {
		return "", fmt.Errorf("no Velocity Spark artifact found in Jenkins build")
	}

	downloadURL := fmt.Sprintf("https://ci.lucko.me/job/spark/lastSuccessfulBuild/artifact/%s", velocityArtifact.RelativePath)
	filename := filepath.Base(velocityArtifact.RelativePath)
	destPath := filepath.Join(destDir, filename)

	log.Printf("[Spark] Downloading Spark for Velocity from %s", downloadURL)

	resp, err = i.httpClient.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to download Velocity Spark JAR: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	f, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		os.Remove(destPath)
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return destPath, nil
}

func getLoadersForServerType(serverType string) []string {
	switch serverType {
	case "fabric":
		return []string{"fabric"}
	case "neoforge":
		return []string{"neoforge"}
	case "forge":
		return []string{"forge"}
	default:
		return nil
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
