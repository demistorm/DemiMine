package mc

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type PaperBuildInfo struct {
	Downloads struct {
		Application struct {
			Name string `json:"name"`
		} `json:"application"`
	} `json:"downloads"`
}

func DownloadPaperJar(version, destPath string) (int, string, error) {
	buildResp, err := fillGet(fmt.Sprintf("%s/projects/paper/versions/%s/builds", fillAPIBaseURL, version))
	if err != nil {
		return 0, "", fmt.Errorf("failed to get paper builds: %w", err)
	}
	defer buildResp.Body.Close()

	var builds []FillBuild
	if err := json.NewDecoder(buildResp.Body).Decode(&builds); err != nil {
		return 0, "", fmt.Errorf("failed to decode build response: %w", err)
	}

	if len(builds) == 0 {
		return 0, "", fmt.Errorf("no builds available for paper %s", version)
	}

	var latestBuild *FillBuild
	for i := range builds {
		if latestBuild == nil || builds[i].ID > latestBuild.ID {
			if builds[i].Channel == "STABLE" {
				latestBuild = &builds[i]
			}
		}
	}

	if latestBuild == nil {
		latestBuild = &builds[0]
	}

	downloadURL := latestBuild.Downloads["server:default"].URL
	sha256 := latestBuild.Downloads["server:default"].Checksums["sha256"]

	if err := downloadFillFile(downloadURL, destPath); err != nil {
		return 0, "", err
	}

	return latestBuild.ID, sha256, nil
}

func DownloadPurpurJar(version, destPath string) (int, string, error) {
	buildInfoResp, err := httpClient.Get(fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/latest", version))
	if err != nil {
		return 0, "", fmt.Errorf("failed to get purpur build info: %w", err)
	}
	defer buildInfoResp.Body.Close()

	var buildInfo struct {
		Build string `json:"build"`
		Hash  string `json:"md5"`
	}
	if err := json.NewDecoder(buildInfoResp.Body).Decode(&buildInfo); err != nil {
		return 0, "", fmt.Errorf("failed to decode purpur build hash: %w", err)
	}

	build, err := strconv.Atoi(buildInfo.Build)
	if err != nil {
		return 0, "", fmt.Errorf("failed to parse purpur build number: %w", err)
	}

	downloadURL := fmt.Sprintf(
		"https://api.purpurmc.org/v2/purpur/%s/latest/download",
		version,
	)

	if err := downloadFile(downloadURL, destPath); err != nil {
		return 0, "", err
	}

	return build, buildInfo.Hash, nil
}

func DownloadFabricInstaller(serverDir string) error {
	installerURL := "https://maven.fabricmc.net/net/fabricmc/fabric-installer/1.0.1/fabric-installer-1.0.1.jar"
	installerPath := filepath.Join(serverDir, "fabric-installer.jar")
	return downloadFile(installerURL, installerPath)
}

func DownloadNeoForgeInstaller(version, serverDir string) error {
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
	return downloadFile(downloadURL, installerPath)
}

func DownloadForgeInstaller(version, serverDir string) error {
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
	return downloadFile(downloadURL, installerPath)
}

func DownloadNanoLimboJar(version, destPath string) (int, string, error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://api.github.com/repos/BoomEaro/NanoLimbo/releases/tags/%s", version))
	if err != nil {
		return 0, "", fmt.Errorf("failed to get nanolimbo release info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("nanolimbo version %s not found", version)
	}

	var release struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return 0, "", fmt.Errorf("failed to decode nanolimbo release: %w", err)
	}

	var downloadURL string
	for _, asset := range release.Assets {
		if strings.HasSuffix(asset.Name, ".jar") && !strings.Contains(asset.Name, "sources") {
			downloadURL = asset.URL
			break
		}
	}

	if downloadURL == "" {
		return 0, "", fmt.Errorf("no JAR file found for nanolimbo %s", version)
	}

	if err := downloadFile(downloadURL, destPath); err != nil {
		return 0, "", err
	}

	return 0, "", nil
}

func DownloadServerJar(serverType, version, destPath string) (int, string, error) {
	serverDir := filepath.Dir(destPath)
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		return 0, "", fmt.Errorf("failed to create directory: %w", err)
	}

	switch serverType {
	case "paper":
		build, hash, err := DownloadPaperJar(version, destPath)
		return build, hash, err
	case "purpur":
		build, hash, err := DownloadPurpurJar(version, destPath)
		return build, hash, err
	case "fabric":
		err := DownloadFabricInstaller(serverDir)
		return 0, "", err
	case "neoforge":
		err := DownloadNeoForgeInstaller(version, serverDir)
		return 0, "", err
	case "forge":
		err := DownloadForgeInstaller(version, serverDir)
		return 0, "", err
	case "nanolimbo":
		build, hash, err := DownloadNanoLimboJar(version, destPath)
		return build, hash, err
	default:
		return 0, "", fmt.Errorf("unsupported server type: %s", serverType)
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
