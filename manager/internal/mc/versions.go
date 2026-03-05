package mc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type VersionInfo struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Builds  int    `json:"builds"`
}

type PaperVersionResponse struct {
	ProjectID   string   `json:"project_id"`
	ProjectName string   `json:"project_name"`
	VersionGroups []string `json:"version_groups"`
	Versions    []string `json:"versions"`
}

type PaperBuildResponse struct {
	ProjectID   string `json:"project_id"`
	ProjectName string `json:"project_name"`
	Version     string `json:"version"`
	Builds      []int  `json:"builds"`
}

type PurpurVersionResponse struct {
	Versions []string `json:"versions"`
}

type FabricVersionResponse struct {
	GameVersions []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	} `json:"-"`
}

type NeoForgeVersionResponse struct {
	Versions []string `json:"versions"`
}

func GetPaperVersions() ([]VersionInfo, error) {
	resp, err := httpClient.Get("https://api.papermc.io/v2/projects/paper")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch paper versions: %w", err)
	}
	defer resp.Body.Close()

	var data PaperVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode paper response: %w", err)
	}

	var versions []VersionInfo
	for _, v := range data.Versions {
		buildResp, err := httpClient.Get(fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s", v))
		if err != nil {
			continue
		}
		
		var buildData PaperBuildResponse
		if err := json.NewDecoder(buildResp.Body).Decode(&buildData); err != nil {
			buildResp.Body.Close()
			continue
		}
		buildResp.Body.Close()

		versions = append(versions, VersionInfo{
			Version: v,
			Stable:  true,
			Builds:  len(buildData.Builds),
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

func GetPurpurVersions() ([]VersionInfo, error) {
	resp, err := httpClient.Get("https://api.purpurmc.org/v2/purpur")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch purpur versions: %w", err)
	}
	defer resp.Body.Close()

	var data PurpurVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode purpur response: %w", err)
	}

	var versions []VersionInfo
	for _, v := range data.Versions {
		versions = append(versions, VersionInfo{
			Version: v,
			Stable:  true,
			Builds:  1,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

func GetFabricVersions() ([]VersionInfo, error) {
	resp, err := httpClient.Get("https://meta.fabricmc.net/v2/versions/game")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch fabric versions: %w", err)
	}
	defer resp.Body.Close()

	var data []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode fabric response: %w", err)
	}

	var versions []VersionInfo
	for _, v := range data {
		versions = append(versions, VersionInfo{
			Version: v.Version,
			Stable:  v.Stable,
			Builds:  1,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

func GetNeoForgeVersions() ([]VersionInfo, error) {
	resp, err := httpClient.Get("https://maven.neoforged.net/api/maven/versions/releases/net/neoforged/neoforge")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch neoforge versions: %w", err)
	}
	defer resp.Body.Close()

	var data NeoForgeVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode neoforge response: %w", err)
	}

	versionMap := make(map[string]VersionInfo)
	for _, v := range data.Versions {
		mcVersion := extractMCVersionFromNeoForge(v)
		if mcVersion == "" {
			continue
		}
		if existing, ok := versionMap[mcVersion]; !ok || !existing.Stable {
			versionMap[mcVersion] = VersionInfo{
				Version: mcVersion,
				Stable:  true,
				Builds:  existing.Builds + 1,
			}
		} else {
			existing.Builds++
			versionMap[mcVersion] = existing
		}
	}

	var versions []VersionInfo
	for _, v := range versionMap {
		versions = append(versions, v)
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

func GetForgeVersions() ([]VersionInfo, error) {
	resp, err := httpClient.Get("https://files.minecraftforge.net/net/minecraftforge/forge/promotions_slim.json")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch forge versions: %w", err)
	}
	defer resp.Body.Close()

	var data struct {
		Promos map[string]string `json:"promos"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode forge response: %w", err)
	}

	versionSet := make(map[string]bool)
	for promo := range data.Promos {
		mcVersion := strings.Split(promo, "-")[0]
		if strings.HasPrefix(mcVersion, "1.") {
			versionSet[mcVersion] = true
		}
	}

	var versions []VersionInfo
	for v := range versionSet {
		versions = append(versions, VersionInfo{
			Version: v,
			Stable:  !strings.Contains(v, "pre") && !strings.Contains(v, "rc"),
			Builds:  1,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	return versions, nil
}

func GetVersions(serverType string) ([]VersionInfo, error) {
	switch strings.ToLower(serverType) {
	case "paper":
		return GetPaperVersions()
	case "purpur":
		return GetPurpurVersions()
	case "fabric":
		return GetFabricVersions()
	case "neoforge":
		return GetNeoForgeVersions()
	case "forge":
		return GetForgeVersions()
	default:
		return nil, fmt.Errorf("unsupported server type: %s", serverType)
	}
}

func extractMCVersionFromNeoForge(neoforgeVersion string) string {
	parts := strings.Split(neoforgeVersion, ".")
	if len(parts) < 2 {
		return ""
	}
	
	if strings.HasPrefix(neoforgeVersion, "1.") {
		if len(parts) >= 2 {
			return "1." + parts[1]
		}
	}
	return ""
}

func compareVersions(a, b string) int {
	aParts := strings.Split(strings.TrimPrefix(a, "1."), ".")
	bParts := strings.Split(strings.TrimPrefix(b, "1."), ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		var aNum, bNum int
		if i < len(aParts) {
			fmt.Sscanf(aParts[i], "%d", &aNum)
		}
		if i < len(bParts) {
			fmt.Sscanf(bParts[i], "%d", &bNum)
		}

		if aNum > bNum {
			return 1
		} else if aNum < bNum {
			return -1
		}
	}

	return 0
}
