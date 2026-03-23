package modrinth

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
)

type SearchParams struct {
	Query  string
	Limit  int
	Offset int
	Facets [][]string
	SortBy string
}

type SearchResult struct {
	Hits      []ProjectHit `json:"hits"`
	Offset    int          `json:"offset"`
	Limit     int          `json:"limit"`
	TotalHits int          `json:"total_hits"`
}

type ProjectHit struct {
	ProjectID       string   `json:"project_id"`
	Slug            string   `json:"slug"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	Categories      []string `json:"categories"`
	ClientSide      string   `json:"client_side"`
	ServerSide      string   `json:"server_side"`
	ProjectType     string   `json:"project_type"`
	Downloads       int      `json:"downloads"`
	Follows         int      `json:"follows"`
	IconURL         string   `json:"icon_url"`
	DateCreated     string   `json:"date_created"`
	DateModified    string   `json:"date_modified"`
	LatestVersion   string   `json:"latest_version"`
	License         string   `json:"license"`
	Gallery         []string `json:"gallery"`
	FeaturedGallery string   `json:"featured_gallery"`
	Color           int      `json:"color"`
	Loaders         []string `json:"loaders"`
	GameVersions    []string `json:"game_versions"`
}

func (c *Client) SearchPlugins(params SearchParams) (*SearchResult, error) {
	query := url.Values{}

	if params.Query != "" {
		query.Set("query", params.Query)
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	} else {
		query.Set("limit", "20")
	}
	if params.Offset > 0 {
		query.Set("offset", strconv.Itoa(params.Offset))
	}
	if params.SortBy != "" {
		query.Set("index", params.SortBy)
	}

	if len(params.Facets) > 0 {
		facetsJSON, err := json.Marshal(params.Facets)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal facets: %w", err)
		}
		query.Set("facets", string(facetsJSON))
	}

	endpoint := "/search?" + query.Encode()
	data, err := c.get(endpoint)
	if err != nil {
		return nil, err
	}

	var result SearchResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}

	return &result, nil
}

func BuildPluginFacets(loaders []string, gameVersion string) [][]string {
	var facets [][]string

	for _, loader := range loaders {
		facets = append(facets, []string{fmt.Sprintf("categories:%s", loader)})
	}

	if gameVersion != "" {
		facets = append(facets, []string{fmt.Sprintf("versions:%s", gameVersion)})
	}

	facets = append(facets, []string{"project_type:plugin"})

	return facets
}

func GetLoaderForServerType(serverType string) []string {
	switch serverType {
	case "paper":
		return []string{"paper", "spigot", "bukkit"}
	case "purpur":
		return []string{"purpur", "paper", "spigot", "bukkit"}
	case "fabric":
		return []string{"fabric"}
	case "neoforge":
		return []string{"neoforge"}
	case "forge":
		return []string{"forge"}
	case "velocity":
		return []string{"velocity"}
	default:
		return nil
	}
}
