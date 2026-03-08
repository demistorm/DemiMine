package mc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

type PaperBuildInfo struct {
	Downloads struct {
		Application struct {
			Name string `json:"name"`
		} `json:"application"`
	} `json:"downloads"`
}

func DownloadPaperJar(version, destPath string) error {
	buildResp, err := httpClient.Get(fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s", version))
	if err != nil {
		return fmt.Errorf("failed to get paper builds: %w", err)
	}
	defer buildResp.Body.Close()

	var buildData PaperBuildResponse
	if err := json.NewDecoder(buildResp.Body).Decode(&buildData); err != nil {
		return fmt.Errorf("failed to decode build response: %w", err)
	}

	if len(buildData.Builds) == 0 {
		return fmt.Errorf("no builds available for paper %s", version)
	}

	latestBuild := buildData.Builds[0]
	for _, b := range buildData.Builds {
		if b > latestBuild {
			latestBuild = b
		}
	}

	downloadURL := fmt.Sprintf(
		"https://api.papermc.io/v2/projects/paper/versions/%s/builds/%d/downloads/%s-%s-%d.jar",
		version, latestBuild, "paper", version, latestBuild,
	)

	return downloadFile(downloadURL, destPath)
}

func DownloadPurpurJar(version, destPath string) error {
	downloadURL := fmt.Sprintf(
		"https://api.purpurmc.org/v2/purpur/%s/latest/download",
		version,
	)
	return downloadFile(downloadURL, destPath)
}

type FabricLoaderVersion struct {
	URL     string `json:"url"`
	Version string `json:"version"`
}

func DownloadFabricJar(version, destPath string) error {
	loaderResp, err := httpClient.Get("https://meta.fabricmc.net/v2/versions/loader")
	if err != nil {
		return fmt.Errorf("failed to get fabric loader versions: %w", err)
	}
	defer loaderResp.Body.Close()

	var loaders []FabricLoaderVersion
	if err := json.NewDecoder(loaderResp.Body).Decode(&loaders); err != nil {
		return fmt.Errorf("failed to decode loader response: %w", err)
	}

	if len(loaders) == 0 {
		return fmt.Errorf("no fabric loader versions available")
	}

	loaderVersion := loaders[0].Version

	installerURL := fmt.Sprintf(
		"https://meta.fabricmc.net/v2/versions/loader/%s/%s/server/jar",
		version, loaderVersion,
	)

	return downloadFile(installerURL, destPath)
}

func DownloadNeoForgeJar(version, serverDir string) error {
	versionsResp, err := httpClient.Get("https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge")
	if err != nil {
		return fmt.Errorf("failed to get neoforge versions: %w", err)
	}
	defer versionsResp.Body.Close()

	var data NeoForgeVersionResponse
	if err := json.NewDecoder(versionsResp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode neoforge response: %w", err)
	}

	var latestVersion string
	for _, v := range data.Versions {
		mcVer := extractMCVersionFromNeoForge(v)
		if mcVer == version {
			latestVersion = v
			break
		}
	}

	if latestVersion == "" {
		return fmt.Errorf("no neoforge version found for mc %s", version)
	}

	downloadURL := fmt.Sprintf(
		"https://maven.neoforged.net/releases/net/neoforged/neoforge/%s/neoforge-%s-installer.jar",
		latestVersion, latestVersion,
	)

	installerPath := filepath.Join(serverDir, "neoforge-installer.jar")
	if err := downloadFile(downloadURL, installerPath); err != nil {
		return err
	}
	defer os.Remove(installerPath)

	cmd := exec.Command("java", "-jar", installerPath, "--installServer")
	cmd.Dir = serverDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("neoforge installer failed: %w", err)
	}

	return nil
}

func DownloadForgeJar(version, serverDir string) error {
	promosResp, err := httpClient.Get("https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json")
	if err != nil {
		return fmt.Errorf("failed to get forge promotions: %w", err)
	}
	defer promosResp.Body.Close()

	var data struct {
		Promos map[string]string `json:"promos"`
	}
	if err := json.NewDecoder(promosResp.Body).Decode(&data); err != nil {
		return fmt.Errorf("failed to decode forge promotions: %w", err)
	}

	forgeVersion, ok := data.Promos[version+"-latest"]
	if !ok {
		forgeVersion, ok = data.Promos[version+"-recommended"]
		if !ok {
			return fmt.Errorf("no forge version found for mc %s", version)
		}
	}

	downloadURL := fmt.Sprintf(
		"https://maven.minecraftforge.net/net/minecraftforge/forge/%s-%s/forge-%s-%s-installer.jar",
		version, forgeVersion, version, forgeVersion,
	)

	installerPath := filepath.Join(serverDir, "forge-installer.jar")
	if err := downloadFile(downloadURL, installerPath); err != nil {
		return err
	}
	defer os.Remove(installerPath)

	cmd := exec.Command("java", "-jar", installerPath, "--installServer")
	cmd.Dir = serverDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("forge installer failed: %w", err)
	}

	return nil
}

func DownloadServerJar(serverType, version, destPath string) error {
	serverDir := filepath.Dir(destPath)
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	switch serverType {
	case "paper":
		return DownloadPaperJar(version, destPath)
	case "purpur":
		return DownloadPurpurJar(version, destPath)
	case "fabric":
		return DownloadFabricJar(version, destPath)
	case "neoforge":
		return DownloadNeoForgeJar(version, serverDir)
	case "forge":
		return DownloadForgeJar(version, serverDir)
	default:
		return fmt.Errorf("unsupported server type: %s", serverType)
	}
}

func downloadFile(url, destPath string) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}
