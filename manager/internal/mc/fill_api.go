package mc

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

const (
	userAgent      = "DemiMine/1.0.0 (https://github.com/demimine)"
	fillAPIBaseURL = "https://fill.papermc.io/v3"
)

type FillDownload struct {
	Name      string            `json:"name"`
	URL       string            `json:"url"`
	Checksums map[string]string `json:"checksums"`
	Size      int64             `json:"size"`
}

type FillBuild struct {
	ID        int                     `json:"id"`
	Time      string                  `json:"time"`
	Channel   string                  `json:"channel"`
	Downloads map[string]FillDownload `json:"downloads"`
}

type FillProjectResponse struct {
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	Versions map[string][]string `json:"versions"`
}

type FillVersionsResponse struct {
	Project struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"project"`
	Version string      `json:"version"`
	Builds  []FillBuild `json:"builds"`
}

func fillGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)
	return httpClient.Do(req)
}

func downloadFillFile(url, destPath string) error {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := httpClient.Do(req)
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
