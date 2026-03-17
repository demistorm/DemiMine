package mc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

type VersionInfo struct {
	Version string `json:"version"`
	Stable  bool   `json:"stable"`
	Builds  int    `json:"builds"`
}

type cachedVersions struct {
	versions  []VersionInfo
	fetchedAt time.Time
	mu        sync.RWMutex
}

const cacheTTL = 10 * time.Minute

func (c *cachedVersions) get() ([]VersionInfo, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if time.Since(c.fetchedAt) < cacheTTL && c.versions != nil {
		return c.versions, true
	}
	return nil, false
}

func (c *cachedVersions) set(versions []VersionInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.versions = versions
	c.fetchedAt = time.Now()
}

var (
	paperCache     = &cachedVersions{}
	purpurCache    = &cachedVersions{}
	fabricCache    = &cachedVersions{}
	neoforgeCache  = &cachedVersions{}
	nanolimboCache = &cachedVersions{}
	forgeCache     = &cachedVersions{}
)

type PaperVersionResponse struct {
	ProjectID     string   `json:"project_id"`
	ProjectName   string   `json:"project_name"`
	VersionGroups []string `json:"version_groups"`
	Versions      []string `json:"versions"`
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
	if versions, ok := paperCache.get(); ok {
		return versions, nil
	}

	resp, err := httpClient.Get("https://api.papermc.io/v2/projects/paper")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch paper versions: %w", err)
	}
	defer resp.Body.Close()

	var data PaperVersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode paper response: %w", err)
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	versions := make([]VersionInfo, 0, len(data.Versions))

	for _, v := range data.Versions {
		wg.Add(1)
		go func(version string) {
			defer wg.Done()

			buildResp, err := httpClient.Get(fmt.Sprintf("https://api.papermc.io/v2/projects/paper/versions/%s", version))
			if err != nil {
				return
			}
			defer buildResp.Body.Close()

			var buildData PaperBuildResponse
			if err := json.NewDecoder(buildResp.Body).Decode(&buildData); err != nil {
				return
			}

			mu.Lock()
			versions = append(versions, VersionInfo{
				Version: version,
				Stable:  true,
				Builds:  len(buildData.Builds),
			})
			mu.Unlock()
		}(v)
	}
	wg.Wait()

	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i].Version, versions[j].Version) > 0
	})

	paperCache.set(versions)
	return versions, nil
}

func GetPurpurVersions() ([]VersionInfo, error) {
	if versions, ok := purpurCache.get(); ok {
		return versions, nil
	}

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

	purpurCache.set(versions)
	return versions, nil
}

func GetFabricVersions() ([]VersionInfo, error) {
	if versions, ok := fabricCache.get(); ok {
		return versions, nil
	}

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

	fabricCache.set(versions)
	return versions, nil
}

func GetNeoForgeVersions() ([]VersionInfo, error) {
	if versions, ok := neoforgeCache.get(); ok {
		return versions, nil
	}

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

	neoforgeCache.set(versions)
	return versions, nil
}

func GetForgeVersions() ([]VersionInfo, error) {
	if versions, ok := forgeCache.get(); ok {
		return versions, nil
	}

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

	forgeCache.set(versions)
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
	case "nanolimbo":
		return GetNanoLimboVersions()
	default:
		return nil, fmt.Errorf("unsupported server type: %s", serverType)
	}
}

func extractMCVersionFromNeoForge(neoforgeVersion string) string {
	// Handle craftmine: "0.25w14craftmine.3-beta" -> "25w14craftmine"
	if strings.Contains(neoforgeVersion, "craftmine") {
		// Find the craftmine version pattern like "25w14craftmine"
		if idx := strings.Index(neoforgeVersion, "craftmine"); idx > 0 {
			start := idx
			for start > 0 && (neoforgeVersion[start-1] >= '0' && neoforgeVersion[start-1] <= '9' || neoforgeVersion[start-1] == 'w') {
				start--
			}
			return neoforgeVersion[start : idx+9] // "craftmine" is 9 chars
		}
		return ""
	}

	// Handle weekly snapshots: "0.25w14a.1-beta" -> "25w14a"
	if strings.Contains(neoforgeVersion, "w") && strings.Contains(neoforgeVersion, "-") {
		// Pattern: 0.XXwYYa.Z-beta
		parts := strings.Split(neoforgeVersion, ".")
		if len(parts) >= 2 && strings.Contains(parts[1], "w") {
			// Extract just the snapshot part like "25w14a"
			snapshotPart := parts[1]
			if idx := strings.Index(snapshotPart, "-"); idx > 0 {
				return snapshotPart[:idx]
			}
			return snapshotPart
		}
	}

	baseVersion := strings.Split(neoforgeVersion, "-")[0]
	parts := strings.Split(baseVersion, ".")

	if len(parts) < 2 {
		return ""
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return ""
	}

	// New MC versioning (26.x+): "26.1.0.0-alpha.1+snapshot-1" -> "26.1-snapshot-1" or "26.1"
	if major >= 26 {
		mcVer := fmt.Sprintf("%s.%s", parts[0], parts[1])
		// Check for snapshot suffix in the original version string
		if idx := strings.Index(neoforgeVersion, "+snapshot-"); idx != -1 {
			snapshotPart := neoforgeVersion[idx+10:]
			return mcVer + "-snapshot-" + snapshotPart
		}
		return mcVer
	}

	// Standard NeoForge format: "21.9.15" -> "1.21.9", "21.0.10" -> "1.21"
	if major >= 14 && major <= 25 {
		if parts[1] == "0" {
			return fmt.Sprintf("1.%s", parts[0])
		}
		return fmt.Sprintf("1.%s.%s", parts[0], parts[1])
	}

	return ""
}

func compareVersions(a, b string) int {
	aSnapshot := extractSnapshotNum(a)
	bSnapshot := extractSnapshotNum(b)
	aBase := strings.Split(a, "-snapshot-")[0]
	bBase := strings.Split(b, "-snapshot-")[0]

	aParts := strings.Split(strings.TrimPrefix(aBase, "1."), ".")
	bParts := strings.Split(strings.TrimPrefix(bBase, "1."), ".")

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

	// Same base version, compare snapshot numbers (non-snapshot > snapshot)
	if aSnapshot == 0 && bSnapshot > 0 {
		return 1
	}
	if aSnapshot > 0 && bSnapshot == 0 {
		return -1
	}
	if aSnapshot > bSnapshot {
		return 1
	} else if aSnapshot < bSnapshot {
		return -1
	}

	return 0
}

func extractSnapshotNum(version string) int {
	if idx := strings.Index(version, "-snapshot-"); idx != -1 {
		var num int
		fmt.Sscanf(version[idx+10:], "%d", &num)
		return num
	}
	return 0
}

type NanoLimboRelease struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func GetNanoLimboVersions() ([]VersionInfo, error) {
	if versions, ok := nanolimboCache.get(); ok {
		return versions, nil
	}

	resp, err := httpClient.Get("https://api.github.com/repos/BoomEaro/NanoLimbo/releases")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nanolimbo versions: %w", err)
	}
	defer resp.Body.Close()

	var releases []NanoLimboRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode nanolimbo response: %w", err)
	}

	var versions []VersionInfo
	for _, release := range releases {
		versions = append(versions, VersionInfo{
			Version: release.TagName,
			Stable:  true,
			Builds:  len(release.Assets),
		})
	}

	nanolimboCache.set(versions)
	return versions, nil
}
