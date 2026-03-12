package mc

import (
	"encoding/json"
	"fmt"
	"net/url"
)

const modrinthAPI = "https://api.modrinth.com/v2"

type ModrinthVersion struct {
	ID            string   `json:"id"`
	VersionNumber string   `json:"version_number"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Files         []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
	} `json:"files"`
}

func DownloadModrinthMod(slug, mcVersion, loader, destPath string) error {
	params := url.Values{}
	gameVersions := fmt.Sprintf(`["%s"]`, mcVersion)
	loaders := fmt.Sprintf(`["%s"]`, loader)
	params.Set("game_versions", gameVersions)
	params.Set("loaders", loaders)

	apiURL := fmt.Sprintf("%s/project/%s/version?%s", modrinthAPI, slug, params.Encode())

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return fmt.Errorf("failed to fetch mod versions: %w", err)
	}
	defer resp.Body.Close()

	var versions []ModrinthVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return fmt.Errorf("failed to decode mod versions: %w", err)
	}

	if len(versions) == 0 {
		return fmt.Errorf("no version found for %s with MC %s and loader %s", slug, mcVersion, loader)
	}

	var downloadURL string
	var filename string
	for _, v := range versions {
		for _, f := range v.Files {
			if f.Primary {
				downloadURL = f.URL
				filename = f.Filename
				break
			}
		}
		if downloadURL != "" {
			break
		}
	}

	if downloadURL == "" && len(versions) > 0 && len(versions[0].Files) > 0 {
		downloadURL = versions[0].Files[0].URL
		filename = versions[0].Files[0].Filename
	}

	if downloadURL == "" {
		return fmt.Errorf("no download file found for %s", slug)
	}

	if err := downloadFile(downloadURL, destPath); err != nil {
		return fmt.Errorf("failed to download %s: %w", filename, err)
	}

	return nil
}

func GetModrinthModFilename(slug, mcVersion, loader string) (string, error) {
	params := url.Values{}
	gameVersions := fmt.Sprintf(`["%s"]`, mcVersion)
	loaders := fmt.Sprintf(`["%s"]`, loader)
	params.Set("game_versions", gameVersions)
	params.Set("loaders", loaders)

	apiURL := fmt.Sprintf("%s/project/%s/version?%s", modrinthAPI, slug, params.Encode())

	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch mod versions: %w", err)
	}
	defer resp.Body.Close()

	var versions []ModrinthVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return "", fmt.Errorf("failed to decode mod versions: %w", err)
	}

	if len(versions) == 0 {
		return "", fmt.Errorf("no version found for %s with MC %s and loader %s", slug, mcVersion, loader)
	}

	for _, v := range versions {
		for _, f := range v.Files {
			if f.Primary {
				return f.Filename, nil
			}
		}
	}

	if len(versions[0].Files) > 0 {
		return versions[0].Files[0].Filename, nil
	}

	return "", fmt.Errorf("no file found for %s", slug)
}
