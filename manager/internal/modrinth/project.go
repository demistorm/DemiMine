package modrinth

import (
	"encoding/json"
	"fmt"
	"net/url"
)

type Project struct {
	ID           string         `json:"id"`
	Slug         string         `json:"slug"`
	Title        string         `json:"title"`
	Description  string         `json:"description"`
	Categories   []string       `json:"categories"`
	ClientSide   string         `json:"client_side"`
	ServerSide   string         `json:"server_side"`
	Body         string         `json:"body"`
	ProjectType  string         `json:"project_type"`
	Team         string         `json:"team"`
	License      License        `json:"license"`
	Downloads    int            `json:"downloads"`
	Followers    int            `json:"followers"`
	IconURL      string         `json:"icon_url"`
	DateCreated  string         `json:"date_created"`
	DateModified string         `json:"date_modified"`
	Gallery      []GalleryImage `json:"gallery"`
}

type License struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

type GalleryImage struct {
	URL      string `json:"url"`
	Featured bool   `json:"featured"`
	Title    string `json:"title"`
	Created  string `json:"created"`
}

type Version struct {
	ID              string        `json:"id"`
	ProjectID       string        `json:"project_id"`
	Name            string        `json:"name"`
	VersionNumber   string        `json:"version_number"`
	Changelog       string        `json:"changelog"`
	GameVersions    []string      `json:"game_versions"`
	Loaders         []string      `json:"loaders"`
	Dependencies    []Dependency  `json:"dependencies"`
	Files           []VersionFile `json:"files"`
	DateCreated     string        `json:"date_created"`
	DateModified    string        `json:"date_modified"`
	Featured        bool          `json:"featured"`
	Status          string        `json:"status"`
	RequestedStatus string        `json:"requested_status"`
	ReleaseChannel  string        `json:"version_type"`
}

type Dependency struct {
	VersionID      string `json:"version_id"`
	ProjectID      string `json:"project_id"`
	FileID         int    `json:"file_id"`
	DependencyType string `json:"dependency_type"`
}

type VersionFile struct {
	Hashes   FileHashes `json:"hashes"`
	URL      string     `json:"url"`
	Filename string     `json:"filename"`
	Primary  bool       `json:"primary"`
	Size     int        `json:"size"`
	FileType string     `json:"file_type"`
}

type FileHashes struct {
	SHA1   string `json:"sha1"`
	SHA512 string `json:"sha512"`
}

func (c *Client) GetProject(slugOrID string) (*Project, error) {
	data, err := c.get("/project/" + slugOrID)
	if err != nil {
		return nil, err
	}

	var project Project
	if err := json.Unmarshal(data, &project); err != nil {
		return nil, fmt.Errorf("failed to parse project: %w", err)
	}

	return &project, nil
}

func (c *Client) GetProjectVersions(slugOrID string, gameVersions []string, loaders []string) ([]Version, error) {
	endpoint := "/project/" + slugOrID + "/version"

	if len(gameVersions) > 0 || len(loaders) > 0 {
		params := url.Values{}
		if len(gameVersions) > 0 {
			gvJSON, _ := json.Marshal(gameVersions)
			params.Set("game_versions", string(gvJSON))
		}
		if len(loaders) > 0 {
			lJSON, _ := json.Marshal(loaders)
			params.Set("loaders", string(lJSON))
		}
		endpoint = endpoint + "?" + params.Encode()
	}

	data, err := c.get(endpoint)
	if err != nil {
		return nil, err
	}

	var versions []Version
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions: %w", err)
	}

	return versions, nil
}

func (c *Client) GetVersion(versionID string) (*Version, error) {
	data, err := c.get("/version/" + versionID)
	if err != nil {
		return nil, err
	}

	var version Version
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, fmt.Errorf("failed to parse version: %w", err)
	}

	return &version, nil
}

func (c *Client) GetVersions(versionIDs []string) ([]Version, error) {
	idsJSON, err := json.Marshal(versionIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal version ids: %w", err)
	}

	data, err := c.get("/versions?ids=" + string(idsJSON))
	if err != nil {
		return nil, err
	}

	var versions []Version
	if err := json.Unmarshal(data, &versions); err != nil {
		return nil, fmt.Errorf("failed to parse versions: %w", err)
	}

	return versions, nil
}

func FindBestVersion(versions []Version, gameVersion string, loaders []string) *Version {
	var best *Version

	for i := range versions {
		v := &versions[i]

		if v.Status != "listed" && v.Status != "archived" {
			continue
		}

		gameMatch := false
		for _, gv := range v.GameVersions {
			if gv == gameVersion {
				gameMatch = true
				break
			}
		}
		if !gameMatch && gameVersion != "" {
			continue
		}

		loaderMatch := false
		for _, l := range v.Loaders {
			for _, target := range loaders {
				if l == target {
					loaderMatch = true
					break
				}
			}
			if loaderMatch {
				break
			}
		}
		if !loaderMatch && len(loaders) > 0 {
			continue
		}

		if best == nil || v.DateCreated > best.DateCreated {
			best = v
		}
	}

	return best
}

func (v *Version) GetPrimaryFile() *VersionFile {
	for i := range v.Files {
		if v.Files[i].Primary {
			return &v.Files[i]
		}
	}
	if len(v.Files) > 0 {
		return &v.Files[0]
	}
	return nil
}
