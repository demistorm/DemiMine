package mc

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

func compareVersionStrings(v1, v2 string) bool {
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	for i := 0; i < len(parts1) && i < len(parts2); i++ {
		n1, err1 := strconv.Atoi(parts1[i])
		n2, err2 := strconv.Atoi(parts2[i])

		if err1 != nil || err2 != nil {
			return v1 > v2
		}

		if n1 > n2 {
			return true
		}
		if n1 < n2 {
			return false
		}
	}

	return len(parts1) > len(parts2)
}

type JarUpdateInfo struct {
	HasUpdate      bool   `json:"has_update"`
	CurrentBuild   int    `json:"current_build"`
	LatestBuild    int    `json:"latest_build"`
	CurrentVersion string `json:"current_version"`
	LatestVersion  string `json:"latest_version"`
}

type cachedUpdate struct {
	info      JarUpdateInfo
	fetchedAt time.Time
	mu        sync.RWMutex
}

const updateCacheTTL = 1 * time.Hour

var (
	paperUpdateCache      = make(map[string]*cachedUpdate)
	paperUpdateCacheMu    sync.RWMutex
	purpurUpdateCache     = make(map[string]*cachedUpdate)
	purpurUpdateCacheMu   sync.RWMutex
	velocityUpdateCache   = make(map[string]*cachedUpdate)
	velocityUpdateCacheMu sync.RWMutex
)

func getPaperCacheKey(version string, currentBuild int) string {
	return fmt.Sprintf("%s-%d", version, currentBuild)
}

func getPurpurCacheKey(version string, currentHash string) string {
	return fmt.Sprintf("%s-%s", version, currentHash)
}

func getVelocityCacheKey(version string, build int) string {
	return fmt.Sprintf("%s-%d", version, build)
}

func ClearPaperCache(version string, build int) {
	cacheKey := getPaperCacheKey(version, build)
	paperUpdateCacheMu.Lock()
	delete(paperUpdateCache, cacheKey)
	paperUpdateCacheMu.Unlock()
}

func ClearVelocityCache(version string, build int) {
	cacheKey := getVelocityCacheKey(version, build)
	velocityUpdateCacheMu.Lock()
	delete(velocityUpdateCache, cacheKey)
	velocityUpdateCacheMu.Unlock()
}

func CheckPaperUpdate(version string, currentBuild int) (*JarUpdateInfo, error) {
	cacheKey := getPaperCacheKey(version, currentBuild)
	paperUpdateCacheMu.RLock()
	cached, exists := paperUpdateCache[cacheKey]
	paperUpdateCacheMu.RUnlock()

	if exists {
		cached.mu.RLock()
		if time.Since(cached.fetchedAt) < updateCacheTTL {
			info := cached.info
			cached.mu.RUnlock()
			return &info, nil
		}
		cached.mu.RUnlock()
	}

	buildResp, err := fillGet(fmt.Sprintf("%s/projects/paper/versions/%s/builds", fillAPIBaseURL, version))
	if err != nil {
		return nil, fmt.Errorf("failed to get paper builds: %w", err)
	}
	defer buildResp.Body.Close()

	var builds []FillBuild
	if err := json.NewDecoder(buildResp.Body).Decode(&builds); err != nil {
		return nil, fmt.Errorf("failed to decode build response: %w", err)
	}

	if len(builds) == 0 {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentBuild:   currentBuild,
			LatestBuild:    currentBuild,
			CurrentVersion: version,
			LatestVersion:  version,
		}, nil
	}

	latestBuild := builds[0]
	for _, b := range builds {
		if b.ID > latestBuild.ID && b.Channel == "STABLE" {
			latestBuild = b
		}
	}

	if latestBuild.Channel != "STABLE" && len(builds) > 1 {
		for _, b := range builds {
			if b.Channel == "STABLE" && b.ID > latestBuild.ID {
				latestBuild = b
			}
		}
	}

	info := JarUpdateInfo{
		HasUpdate:      latestBuild.ID > currentBuild,
		CurrentBuild:   currentBuild,
		LatestBuild:    latestBuild.ID,
		CurrentVersion: version,
		LatestVersion:  version,
	}

	paperUpdateCacheMu.Lock()
	paperUpdateCache[cacheKey] = &cachedUpdate{info: info, fetchedAt: time.Now()}
	paperUpdateCacheMu.Unlock()

	return &info, nil
}

type PurpurVersionDetailResponse struct {
	Version struct {
		Project string `json:"project"`
		Version string `json:"version"`
	} `json:"version"`
	Builds struct {
		Latest string   `json:"latest"`
		All    []string `json:"all"`
	} `json:"builds"`
}

type PurpurBuildInfo struct {
	Hash string `json:"md5"`
}

func CheckPurpurUpdate(version string, currentHash string) (*JarUpdateInfo, error) {
	cacheKey := getPurpurCacheKey(version, currentHash)
	purpurUpdateCacheMu.RLock()
	cached, exists := purpurUpdateCache[cacheKey]
	purpurUpdateCacheMu.RUnlock()

	if exists {
		cached.mu.RLock()
		if time.Since(cached.fetchedAt) < updateCacheTTL {
			info := cached.info
			cached.mu.RUnlock()
			return &info, nil
		}
		cached.mu.RUnlock()
	}

	verResp, err := httpClient.Get(fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s", version))
	if err != nil {
		return nil, fmt.Errorf("failed to get purpur version details: %w", err)
	}
	defer verResp.Body.Close()

	var verData PurpurVersionDetailResponse
	if err := json.NewDecoder(verResp.Body).Decode(&verData); err != nil {
		return nil, fmt.Errorf("failed to decode purpur version response: %w", err)
	}

	if verData.Builds.Latest == "" {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentBuild:   0,
			LatestBuild:    0,
			CurrentVersion: version,
			LatestVersion:  version,
		}, nil
	}

	buildInfoResp, err := httpClient.Get(fmt.Sprintf("https://api.purpurmc.org/v2/purpur/%s/%s", version, verData.Builds.Latest))
	if err != nil {
		return nil, fmt.Errorf("failed to get purpur build info: %w", err)
	}
	defer buildInfoResp.Body.Close()

	var buildInfo struct {
		Hash string `json:"md5"`
	}
	if err := json.NewDecoder(buildInfoResp.Body).Decode(&buildInfo); err != nil {
		return nil, fmt.Errorf("failed to decode purpur build hash: %w", err)
	}

	info := JarUpdateInfo{
		HasUpdate:      currentHash == "" || buildInfo.Hash != currentHash,
		CurrentBuild:   0,
		LatestBuild:    0,
		CurrentVersion: version,
		LatestVersion:  version,
	}

	purpurUpdateCacheMu.Lock()
	purpurUpdateCache[cacheKey] = &cachedUpdate{info: info, fetchedAt: time.Now()}
	purpurUpdateCacheMu.Unlock()

	return &info, nil
}

func CheckVelocityUpdate(currentVersion string, currentBuild int) (*JarUpdateInfo, error) {
	cacheKey := getVelocityCacheKey(currentVersion, currentBuild)
	velocityUpdateCacheMu.RLock()
	cached, exists := velocityUpdateCache[cacheKey]
	velocityUpdateCacheMu.RUnlock()

	if exists {
		cached.mu.RLock()
		if time.Since(cached.fetchedAt) < updateCacheTTL {
			info := cached.info
			cached.mu.RUnlock()
			return &info, nil
		}
		cached.mu.RUnlock()
	}

	resp, err := fillGet(fmt.Sprintf("%s/projects/velocity", fillAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to get velocity versions: %w", err)
	}
	defer resp.Body.Close()

	var data FillProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode velocity versions: %w", err)
	}

	versions, ok := data.Versions["3.0.0"]
	if !ok || len(versions) == 0 {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentBuild:   currentBuild,
			LatestBuild:    currentBuild,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
		}, nil
	}

	var latestVersion string
	var latestBuildID int
	var latestBuildChannel string

	for _, version := range versions {
		buildResp, err := fillGet(fmt.Sprintf("%s/projects/velocity/versions/%s/builds", fillAPIBaseURL, version))
		if err != nil {
			continue
		}

		var builds []FillBuild
		if err := json.NewDecoder(buildResp.Body).Decode(&builds); err != nil {
			buildResp.Body.Close()
			continue
		}
		buildResp.Body.Close()

		if len(builds) == 0 {
			continue
		}

		for _, build := range builds {
			if build.Channel == "STABLE" && (latestBuildChannel != "STABLE" || build.ID > latestBuildID) {
				latestVersion = version
				latestBuildID = build.ID
				latestBuildChannel = "STABLE"
			} else if latestBuildChannel == "" && build.ID > latestBuildID {
				latestVersion = version
				latestBuildID = build.ID
				latestBuildChannel = build.Channel
			}
		}
	}

	if latestVersion == "" {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentBuild:   currentBuild,
			LatestBuild:    currentBuild,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
		}, nil
	}

	hasUpdate := false
	if latestVersion != currentVersion {
		hasUpdate = true
	} else if latestBuildID > currentBuild {
		hasUpdate = true
	}

	info := JarUpdateInfo{
		HasUpdate:      hasUpdate,
		CurrentBuild:   currentBuild,
		LatestBuild:    latestBuildID,
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
	}

	velocityUpdateCacheMu.Lock()
	velocityUpdateCache[cacheKey] = &cachedUpdate{info: info, fetchedAt: time.Now()}
	velocityUpdateCacheMu.Unlock()

	return &info, nil
}
