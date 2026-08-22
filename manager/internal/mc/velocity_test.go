package mc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// mirrors the real fill v3 velocity data shape: versions array in arbitrary
// order, 3.5.0 with zero builds, snapshot builds marked STABLE but with a
// RECOMMENDED 3.5.1 that's actually newer, and a dev build with the highest
// id but a junk channel
func velocityMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/projects/velocity":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"project":{"id":"velocity","name":"Velocity"},"versions":{"3.0.0":["3.5.1","3.5.0","3.6.0-SNAPSHOT","3.5.0-SNAPSHOT"]}}`))
		case "/v3/projects/velocity/versions/3.5.1/builds":
			w.Write([]byte(`[{"id":615,"channel":"RECOMMENDED","downloads":{"server:default":{"name":"velocity.jar","url":"https://fill.papermc.io/v3/velocity/3.5.1/615/download"}}}]`))
		case "/v3/projects/velocity/versions/3.5.0/builds":
			w.Write([]byte(`[]`))
		case "/v3/projects/velocity/versions/3.6.0-SNAPSHOT/builds":
			w.Write([]byte(`[{"id":611,"channel":"STABLE","downloads":{}},{"id":613,"channel":"STABLE","downloads":{}}]`))
		case "/v3/projects/velocity/versions/3.5.0-SNAPSHOT/builds":
			w.Write([]byte(`[{"id":999,"channel":"EXPERIMENTAL","downloads":{}}]`))
		case "/v3/velocity/3.5.1/615/download":
			w.Write([]byte("fake jar bytes"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func withVelocityMock(t *testing.T) func() {
	t.Helper()
	server := velocityMockServer(t)
	oldClient := httpClient
	httpClient = &http.Client{Transport: &testTransport{server.URL}}
	return func() {
		httpClient = oldClient
		server.Close()
	}
}

func TestLatestVelocityBuild(t *testing.T) {
	defer withVelocityMock(t)()

	best, err := latestVelocityBuild()
	if err != nil {
		t.Fatalf("latestVelocityBuild() error = %v", err)
	}
	if best == nil {
		t.Fatal("expected a pick, got nil")
	}
	// RECOMMENDED 3.5.1 build 615 must win over STABLE snapshots (613) and
	// the EXPERIMENTAL build with the highest id (999)
	if best.version != "3.5.1" || best.build.ID != 615 {
		t.Errorf("expected 3.5.1 build 615, got %s build %d (channel %s)", best.version, best.build.ID, best.build.Channel)
	}
}

func TestCheckVelocityUpdate(t *testing.T) {
	velocityUpdateCacheMu.Lock()
	velocityUpdateCache = make(map[string]*cachedUpdate)
	velocityUpdateCacheMu.Unlock()

	defer withVelocityMock(t)()

	// same version, older build -> update available
	info, err := CheckVelocityUpdate("3.5.1", 600)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if !info.HasUpdate || info.LatestVersion != "3.5.1" || info.LatestBuild != 615 {
		t.Errorf("expected update to 3.5.1/615, got %+v", info)
	}

	// fully current -> no update
	info, err = CheckVelocityUpdate("3.5.1", 615)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if info.HasUpdate {
		t.Errorf("expected no update when on 3.5.1/615, got %+v", info)
	}

	// older version -> update available
	info, err = CheckVelocityUpdate("3.5.0", 600)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if !info.HasUpdate || info.LatestVersion != "3.5.1" {
		t.Errorf("expected update from 3.5.0 to 3.5.1, got %+v", info)
	}
}

func TestDownloadVelocityJarPicksSameBuild(t *testing.T) {
	defer withVelocityMock(t)()

	dest := filepath.Join(t.TempDir(), "velocity.jar")
	version, build, err := DownloadVelocityJar(dest)
	if err != nil {
		t.Fatalf("DownloadVelocityJar() error = %v", err)
	}

	if version != "3.5.1" || build != 615 {
		t.Errorf("expected download of 3.5.1/615, got %s/%d", version, build)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("failed to read downloaded jar: %v", err)
	}
	if string(data) != "fake jar bytes" {
		t.Errorf("jar content mismatch, got %q", string(data))
	}

	// the whole point of the fix: download target must match what check reports
	info, err := CheckVelocityUpdate("3.5.0", 600)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if info.LatestVersion != version || info.LatestBuild != build {
		t.Errorf("check (%s/%d) and download (%s/%d) disagree", info.LatestVersion, info.LatestBuild, version, build)
	}
}
