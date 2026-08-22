package mc

import (
	"encoding/json"
	"fmt"
	"net/http"
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

func sweepExpiredFromMap(m map[string]*cachedUpdate, mu *sync.RWMutex) {
	mu.Lock()
	defer mu.Unlock()
	now := time.Now()
	for k, v := range m {
		if now.Sub(v.fetchedAt) > updateCacheTTL {
			delete(m, k)
		}
	}
}

var (
	paperUpdateCache      = make(map[string]*cachedUpdate)
	paperUpdateCacheMu    sync.RWMutex
	purpurUpdateCache     = make(map[string]*cachedUpdate)
	purpurUpdateCacheMu   sync.RWMutex
	velocityUpdateCache   = make(map[string]*cachedUpdate)
	velocityUpdateCacheMu sync.RWMutex
	nanolimboUpdateCache  *cachedUpdate
	nanolimboUpdateMu     sync.RWMutex
)

func getPaperCacheKey(version string, currentBuild int) string {
	return fmt.Sprintf("%s-%d", version, currentBuild)
}

func getPurpurCacheKey(version string, currentBuild int) string {
	return fmt.Sprintf("%s-%d", version, currentBuild)
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

type velocityBuildPick struct {
	version  string
	build    FillBuild
	priority int
}

// fill channels rank how "shippable" a build is — RECOMMENDED is what
// papermc actually points people at, STABLE is fine, everything else is a
// gamble. only used to pick between builds of the *same* version now.
func channelPriority(channel string) int {
	switch channel {
	case "RECOMMENDED":
		return 3
	case "STABLE":
		return 2
	default:
		return 1
	}
}

// velocity version compare: "4.1.0-SNAPSHOT" vs "4.0.0" vs "3.5.1" —
// numeric dot parts, and a -SNAPSHOT suffix loses to the release of the
// same base version but beats any lower version
func velocityVersionLess(a, b string) bool {
	split := func(v string) ([]int, bool) {
		isSnap := strings.HasSuffix(v, "-SNAPSHOT")
		base := strings.TrimSuffix(v, "-SNAPSHOT")
		parts := strings.Split(base, ".")
		nums := make([]int, len(parts))
		for i, p := range parts {
			fmt.Sscanf(p, "%d", &nums[i])
		}
		return nums, isSnap
	}

	aNums, aSnap := split(a)
	bNums, bSnap := split(b)

	maxLen := len(aNums)
	if len(bNums) > maxLen {
		maxLen = len(bNums)
	}
	for i := 0; i < maxLen; i++ {
		var an, bn int
		if i < len(aNums) {
			an = aNums[i]
		}
		if i < len(bNums) {
			bn = bNums[i]
		}
		if an != bn {
			return an < bn
		}
	}
	// same base version — release beats snapshot
	return aSnap && !bSnap
}

// the single source of truth for "what is the latest velocity build" — both
// CheckVelocityUpdate and DownloadVelocityJar go through here so they can
// never disagree. fill groups versions by major family under separate keys
// ("3.0.0", "4.0.0", ...) with random ordering, so flatten everything, and
// match what the papermc site serves as the primary download: the newest
// version line, best build within it. RECOMMENDED tags on old majors must
// NOT keep us pinned to 3.x forever.
func latestVelocityBuild() (*velocityBuildPick, error) {
	resp, err := fillGet(fmt.Sprintf("%s/projects/velocity", fillAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("failed to get velocity versions: %w", err)
	}
	defer resp.Body.Close()

	var data FillProjectResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to decode velocity versions: %w", err)
	}

	// flatten every version group — 4.x lives under a different key than 3.x
	var versions []string
	for _, group := range data.Versions {
		versions = append(versions, group...)
	}
	if len(versions) == 0 {
		return nil, nil
	}

	// newest version first, then walk down until we find one with builds
	for i := 0; i < len(versions); i++ {
		for j := i + 1; j < len(versions); j++ {
			if velocityVersionLess(versions[i], versions[j]) {
				versions[i], versions[j] = versions[j], versions[i]
			}
		}
	}

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

		var best *FillBuild
		bestPriority := 0
		for j := range builds {
			p := channelPriority(builds[j].Channel)
			if best == nil || p > bestPriority || (p == bestPriority && builds[j].ID > best.ID) {
				best = &builds[j]
				bestPriority = p
			}
		}

		if best != nil {
			return &velocityBuildPick{version: version, build: *best, priority: bestPriority}, nil
		}
	}

	return nil, nil
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

	sweepExpiredFromMap(paperUpdateCache, &paperUpdateCacheMu)

	return &info, nil
}

type PurpurVersionDetailResponse struct {
	Project string `json:"project"`
	Version string `json:"version"`
	Builds  struct {
		Latest string   `json:"latest"`
		All    []string `json:"all"`
	} `json:"builds"`
}

type PurpurBuildInfo struct {
	Hash string `json:"md5"`
}

func CheckPurpurUpdate(version string, currentBuild int, currentHash string) (*JarUpdateInfo, error) {
	cacheKey := getPurpurCacheKey(version, currentBuild)
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

	if verResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("purpur version API returned status %d", verResp.StatusCode)
	}

	var verData PurpurVersionDetailResponse
	if err := json.NewDecoder(verResp.Body).Decode(&verData); err != nil {
		return nil, fmt.Errorf("failed to decode purpur version response: %w", err)
	}

	if verData.Builds.Latest == "" {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentBuild:   currentBuild,
			LatestBuild:    0,
			CurrentVersion: version,
			LatestVersion:  version,
		}, nil
	}

	latestBuild, err := strconv.Atoi(verData.Builds.Latest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse purpur latest build number: %w", err)
	}

	info := JarUpdateInfo{
		HasUpdate:      currentBuild == 0 || latestBuild > currentBuild,
		CurrentBuild:   currentBuild,
		LatestBuild:    latestBuild,
		CurrentVersion: version,
		LatestVersion:  version,
	}

	purpurUpdateCacheMu.Lock()
	purpurUpdateCache[cacheKey] = &cachedUpdate{info: info, fetchedAt: time.Now()}
	purpurUpdateCacheMu.Unlock()

	sweepExpiredFromMap(purpurUpdateCache, &purpurUpdateCacheMu)

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

	best, err := latestVelocityBuild()
	if err != nil {
		return nil, err
	}

	if best == nil {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentBuild:   currentBuild,
			LatestBuild:    currentBuild,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
		}, nil
	}

	hasUpdate := false
	if best.version != currentVersion {
		hasUpdate = true
	} else if best.build.ID > currentBuild {
		hasUpdate = true
	}

	info := JarUpdateInfo{
		HasUpdate:      hasUpdate,
		CurrentBuild:   currentBuild,
		LatestBuild:    best.build.ID,
		CurrentVersion: currentVersion,
		LatestVersion:  best.version,
	}

	velocityUpdateCacheMu.Lock()
	velocityUpdateCache[cacheKey] = &cachedUpdate{info: info, fetchedAt: time.Now()}
	velocityUpdateCacheMu.Unlock()

	sweepExpiredFromMap(velocityUpdateCache, &velocityUpdateCacheMu)

	return &info, nil
}

func CheckNanoLimboUpdate(currentVersion string) (*JarUpdateInfo, error) {
	nanolimboUpdateMu.RLock()
	if nanolimboUpdateCache != nil {
		if time.Since(nanolimboUpdateCache.fetchedAt) < updateCacheTTL {
			info := nanolimboUpdateCache.info
			nanolimboUpdateMu.RUnlock()
			return &info, nil
		}
	}
	nanolimboUpdateMu.RUnlock()

	resp, err := httpClient.Get("https://api.github.com/repos/BoomEaro/NanoLimbo/releases")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch nanolimbo releases: %w", err)
	}
	defer resp.Body.Close()

	var releases []NanoLimboRelease
	if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to decode nanolimbo releases: %w", err)
	}

	if len(releases) == 0 {
		return &JarUpdateInfo{
			HasUpdate:      false,
			CurrentVersion: currentVersion,
			LatestVersion:  currentVersion,
		}, nil
	}

	latestVersion := releases[0].TagName

	// strip "v" prefix for comparison
	stripV := func(s string) string {
		return strings.TrimPrefix(s, "v")
	}

	currentClean := stripV(currentVersion)
	latestClean := stripV(latestVersion)

	hasUpdate := compareVersionStrings(latestClean, currentClean)

	info := JarUpdateInfo{
		HasUpdate:      hasUpdate,
		CurrentVersion: currentVersion,
		LatestVersion:  latestVersion,
	}

	nanolimboUpdateMu.Lock()
	nanolimboUpdateCache = &cachedUpdate{info: info, fetchedAt: time.Now()}
	nanolimboUpdateMu.Unlock()

	return &info, nil
}
