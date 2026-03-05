package mc

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b     string
		expected int
	}{
		{"1.21.3", "1.21.2", 1},
		{"1.21.3", "1.21.3", 0},
		{"1.21.2", "1.21.3", -1},
		{"1.20.4", "1.21.0", -1},
		{"1.21", "1.20.4", 1},
		{"1.16.5", "1.17.0", -1},
		{"1.8.9", "1.12.2", -1},
		{"1.21.3", "1.21", 1},
	}

	for _, tt := range tests {
		result := compareVersions(tt.a, tt.b)
		if result != tt.expected {
			t.Errorf("compareVersions(%q, %q) = %d, expected %d", tt.a, tt.b, result, tt.expected)
		}
	}
}

func TestExtractMCVersionFromNeoForge(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.21.3", "1.21"},
		{"1.20.4", "1.20"},
		{"1.19.2", "1.19"},
		{"1.18.2", "1.18"},
		{"1.17.1", "1.17"},
		{"1.16.5", "1.16"},
		{"21.0.0", ""},
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractMCVersionFromNeoForge(tt.input)
		if result != tt.expected {
			t.Errorf("extractMCVersionFromNeoForge(%q) = %q, expected %q", tt.input, tt.expected, result)
		}
	}
}

func TestGetVersionsUnsupported(t *testing.T) {
	_, err := GetVersions("unsupported")
	if err == nil {
		t.Error("GetVersions should return error for unsupported type")
	}
}

func TestGetVersionsSupportedTypes(t *testing.T) {
	types := []string{"paper", "purpur", "fabric", "neoforge", "forge"}
	for _, serverType := range types {
		_, err := GetVersions(serverType)
		if err != nil {
			t.Logf("GetVersions(%q) error (may be network): %v", serverType, err)
		}
	}
}

func TestVersionInfoStruct(t *testing.T) {
	v := VersionInfo{
		Version: "1.21.3",
		Stable:  true,
		Builds:  83,
	}

	if v.Version != "1.21.3" {
		t.Errorf("Version = %q, want %q", v.Version, "1.21.3")
	}
	if !v.Stable {
		t.Error("Stable should be true")
	}
	if v.Builds != 83 {
		t.Errorf("Builds = %d, want 83", v.Builds)
	}
}

func TestGetPaperVersionsMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/projects/paper":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"project_id":"paper","project_name":"Paper","versions":["1.21.3","1.20.4","1.19.4"]}`))
		case "/v2/projects/paper/versions/1.21.3":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"project_id":"paper","version":"1.21.3","builds":[80,81,82,83]}`))
		case "/v2/projects/paper/versions/1.20.4":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"project_id":"paper","version":"1.20.4","builds":[100,101]}`))
		case "/v2/projects/paper/versions/1.19.4":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"project_id":"paper","version":"1.19.4","builds":[50,51]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = &http.Client{
		Transport: &testTransport{server.URL},
	}
	defer func() { httpClient = oldClient }()

	versions, err := GetPaperVersions()
	if err != nil {
		t.Fatalf("GetPaperVersions() error = %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("Expected 3 versions, got %d", len(versions))
	}

	if versions[0].Version != "1.21.3" {
		t.Errorf("First version should be 1.21.3 (highest), got %s", versions[0].Version)
	}

	for _, v := range versions {
		switch v.Version {
		case "1.21.3":
			if v.Builds != 4 {
				t.Errorf("1.21.3 should have 4 builds, got %d", v.Builds)
			}
		case "1.20.4":
			if v.Builds != 2 {
				t.Errorf("1.20.4 should have 2 builds, got %d", v.Builds)
			}
		case "1.19.4":
			if v.Builds != 2 {
				t.Errorf("1.19.4 should have 2 builds, got %d", v.Builds)
			}
		}
	}
}

func TestGetPurpurVersionsMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/purpur" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"versions":["1.21.3","1.20.4","1.19.4"]}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = &http.Client{
		Transport: &testTransport{server.URL},
	}
	defer func() { httpClient = oldClient }()

	versions, err := GetPurpurVersions()
	if err != nil {
		t.Fatalf("GetPurpurVersions() error = %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("Expected 3 versions, got %d", len(versions))
	}

	if versions[0].Version != "1.21.3" {
		t.Errorf("First version should be 1.21.3 (highest), got %s", versions[0].Version)
	}
}

func TestGetFabricVersionsMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/versions/game" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"version":"1.21.3","stable":true},
				{"version":"1.21.2","stable":true},
				{"version":"1.20.4","stable":true},
				{"version":"22w20a","stable":false}
			]`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	oldClient := httpClient
	httpClient = &http.Client{
		Transport: &testTransport{server.URL},
	}
	defer func() { httpClient = oldClient }()

	versions, err := GetFabricVersions()
	if err != nil {
		t.Fatalf("GetFabricVersions() error = %v", err)
	}

	if len(versions) != 4 {
		t.Errorf("Expected 4 versions, got %d", len(versions))
	}

	stableCount := 0
	for _, v := range versions {
		if v.Stable {
			stableCount++
		}
	}
	if stableCount != 3 {
		t.Errorf("Expected 3 stable versions, got %d", stableCount)
	}
}

type testTransport struct {
	baseURL string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "api.papermc.io" || req.URL.Host == "api.purpurmc.org" || req.URL.Host == "meta.fabricmc.net" {
		newURL := t.baseURL + req.URL.Path
		newReq, err := http.NewRequest(req.Method, newURL, req.Body)
		if err != nil {
			return nil, err
		}
		newReq.Header = req.Header
		return http.DefaultClient.Do(newReq)
	}
	return http.DefaultTransport.RoundTrip(req)
}
