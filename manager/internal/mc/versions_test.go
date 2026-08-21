package mc

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
		// Snapshot comparisons
		{"26.1-snapshot-10", "26.1-snapshot-2", 1},
		{"26.1-snapshot-1", "26.1-snapshot-10", -1},
		{"26.1-snapshot-5", "26.1-snapshot-5", 0},
		{"26.1", "26.1-snapshot-10", 1},
		{"26.1-snapshot-10", "26.1", -1},
		{"26.2", "26.1-snapshot-10", 1},
		{"26.1-snapshot-10", "26.2", -1},
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
		// Standard NeoForge format (MC 1.14-1.21.x)
		{"21.0.0-beta", "1.21"},
		{"21.0.167-beta", "1.21"},
		{"21.1.1", "1.21.1"},
		{"21.9.15-beta", "1.21.9"},
		{"21.10.63", "1.21.10"},
		{"21.11.38-beta", "1.21.11"},
		{"20.4.234", "1.20.4"},
		{"20.3.8-beta", "1.20.3"},
		{"19.2.0", "1.19.2"},
		{"18.2.0", "1.18.2"},
		{"17.1.0", "1.17.1"},
		{"16.5.0", "1.16.5"},
		{"14.0.0", "1.14"},
		// New MC versioning (26.x+) with snapshots
		{"26.1.0.0-alpha.1+snapshot-1", "26.1-snapshot-1"},
		{"26.1.0.0-alpha.10+snapshot-6", "26.1-snapshot-6"},
		{"26.1.0.0-alpha.13+snapshot-10", "26.1-snapshot-10"},
		// Craftmine
		{"0.25w14craftmine.3-beta", "25w14craftmine"},
		{"0.25w14craftmine.5-beta", "25w14craftmine"},
		// Edge cases
		{"invalid", ""},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractMCVersionFromNeoForge(tt.input)
		if result != tt.expected {
			t.Errorf("extractMCVersionFromNeoForge(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestGetVersionsUnsupported(t *testing.T) {
	_, err := GetVersions("unsupported", false)
	if err == nil {
		t.Error("GetVersions should return error for unsupported type")
	}
}

func TestGetVersionsSupportedTypes(t *testing.T) {
	types := []string{"paper", "purpur", "fabric", "neoforge", "forge"}
	for _, serverType := range types {
		_, err := GetVersions(serverType, false)
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
	paperCache.versions = nil
	paperCache.fetchedAt = time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/projects/paper" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"project":{"id":"paper","name":"Paper"},"versions":{"1.21":["1.21.3","1.21.2-pre1"],"1.20":["1.20.4"],"1.19":["1.19.4"]}}`))
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

	versions, err := GetPaperVersions(false)
	if err != nil {
		t.Fatalf("GetPaperVersions() error = %v", err)
	}

	// 1.21.2-pre1 survives the filter since no full 1.21.2 release exists in this mock
	if len(versions) != 4 {
		t.Errorf("Expected 4 versions, got %d", len(versions))
	}

	if versions[0].Version != "1.21.3" {
		t.Errorf("First version should be 1.21.3 (highest), got %s", versions[0].Version)
	}

	versionSet := make(map[string]VersionInfo)
	for _, v := range versions {
		versionSet[v.Version] = v
	}

	for _, expected := range []string{"1.21.3", "1.21.2-pre1", "1.20.4", "1.19.4"} {
		if _, ok := versionSet[expected]; !ok {
			t.Errorf("Expected version %q to be present", expected)
		}
	}

	if versionSet["1.21.3"].Stable != true {
		t.Error("1.21.3 should be stable")
	}
	if versionSet["1.21.2-pre1"].Stable != false {
		t.Error("1.21.2-pre1 should not be stable")
	}
}

func TestGetPurpurVersionsMock(t *testing.T) {
	purpurCache.versions = nil
	purpurCache.fetchedAt = time.Time{}
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
	fabricCache.versions = nil
	fabricCache.fetchedAt = time.Time{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/versions/game" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"version":"1.21.3","stable":true},
				{"version":"1.21.2","stable":true},
				{"version":"1.21.2-pre1","stable":false},
				{"version":"1.20.4","stable":true},
				{"version":"25w14craftmine","stable":false},
				{"version":"25w46a","stable":false},
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

	versions, err := GetFabricVersions(false)
	if err != nil {
		t.Fatalf("GetFabricVersions() error = %v", err)
	}

	// 22w20a and 25w46a should be filtered (old-style weekly snapshots)
	// 1.21.2-pre1 should be filtered (full release 1.21.2 exists)
	// 25w14craftmine should be kept (special snapshot)
	// 1.21.3, 1.21.2, 1.20.4 should be kept
	if len(versions) != 4 {
		t.Errorf("Expected 4 versions, got %d", len(versions))
	}

	versionSet := make(map[string]bool)
	for _, v := range versions {
		versionSet[v.Version] = true
	}

	for _, expected := range []string{"1.21.3", "1.21.2", "1.20.4", "25w14craftmine"} {
		if !versionSet[expected] {
			t.Errorf("Expected version %q to be present", expected)
		}
	}
	for _, unexpected := range []string{"22w20a", "25w46a", "1.21.2-pre1"} {
		if versionSet[unexpected] {
			t.Errorf("Expected version %q to be filtered out", unexpected)
		}
	}
}

func TestGetFabricVersionsIncludeAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/versions/game" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`[
				{"version":"1.21.3","stable":true},
				{"version":"1.21.2","stable":true},
				{"version":"1.21.2-pre1","stable":false},
				{"version":"1.20.4","stable":true},
				{"version":"25w14craftmine","stable":false},
				{"version":"25w46a","stable":false},
				{"version":"22w20a","stable":false},
				{"version":"22w18a_unobfuscated","stable":false}
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

	versions, err := GetFabricVersions(true)
	if err != nil {
		t.Fatalf("GetFabricVersions(true) error = %v", err)
	}

	versionSet := make(map[string]bool)
	for _, v := range versions {
		versionSet[v.Version] = true
	}

	for _, expected := range []string{"1.21.3", "1.21.2", "1.21.2-pre1", "1.20.4", "25w14craftmine", "25w46a", "22w20a", "22w18a_unobfuscated"} {
		if !versionSet[expected] {
			t.Errorf("GetFabricVersions(true): expected version %q to be present", expected)
		}
	}
}

func TestGetPaperVersionsIncludeAll(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v3/projects/paper" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"project":{"id":"paper","name":"Paper"},"versions":{"1.21":["1.21.3","1.21.2","1.21.2-pre1"],"1.20":["1.20.4"]}}`))
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

	versions, err := GetPaperVersions(true)
	if err != nil {
		t.Fatalf("GetPaperVersions(true) error = %v", err)
	}

	versionSet := make(map[string]bool)
	for _, v := range versions {
		versionSet[v.Version] = true
	}

	for _, expected := range []string{"1.21.3", "1.21.2", "1.21.2-pre1", "1.20.4"} {
		if !versionSet[expected] {
			t.Errorf("GetPaperVersions(true): expected version %q to be present", expected)
		}
	}
}

type testTransport struct {
	baseURL string
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == "fill.papermc.io" || req.URL.Host == "api.purpurmc.org" || req.URL.Host == "meta.fabricmc.net" {
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
