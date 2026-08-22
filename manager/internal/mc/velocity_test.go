package mc

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// mirrors the real fill v3 velocity data shape: versions grouped under
// separate major-family keys ("3.0.0", "4.0.0") with random ordering inside
// each, 3.5.0 with zero builds, a RECOMMENDED 3.5.1 tag, and the 4.1.0
// dev line that papermc's site serves as the primary download
func velocityMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v3/projects/velocity":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"project":{"id":"velocity","name":"Velocity"},"versions":{"3.0.0":["3.5.1","3.5.0","3.6.0-SNAPSHOT","3.5.0-SNAPSHOT"],"4.0.0":["4.1.0-SNAPSHOT","4.0.0"],"1.0.0":["1.0.10"]}}`))
		case "/v3/projects/velocity/versions/3.5.1/builds":
			w.Write([]byte(`[{"id":615,"channel":"RECOMMENDED","downloads":{"server:default":{"name":"velocity.jar","url":"http://fill.papermc.io/v3/velocity/3.5.1/615/download"}}}]`))
		case "/v3/projects/velocity/versions/3.5.0/builds":
			w.Write([]byte(`[]`))
		case "/v3/projects/velocity/versions/3.6.0-SNAPSHOT/builds":
			w.Write([]byte(`[{"id":611,"channel":"STABLE","downloads":{}},{"id":613,"channel":"STABLE","downloads":{}}]`))
		case "/v3/projects/velocity/versions/3.5.0-SNAPSHOT/builds":
			w.Write([]byte(`[{"id":999,"channel":"EXPERIMENTAL","downloads":{}}]`))
		case "/v3/projects/velocity/versions/4.1.0-SNAPSHOT/builds":
			w.Write([]byte(`[{"id":19,"channel":"STABLE","downloads":{}},{"id":21,"channel":"STABLE","downloads":{"server:default":{"name":"velocity.jar","url":"http://fill.papermc.io/v3/velocity/4.1.0-SNAPSHOT/21/download"}}}]`))
		case "/v3/projects/velocity/versions/4.0.0/builds":
			w.Write([]byte(`[{"id":6,"channel":"STABLE","downloads":{}}]`))
		case "/v3/projects/velocity/versions/1.0.10/builds":
			w.Write([]byte(`[{"id":2,"channel":"STABLE","downloads":{}}]`))
		case "/v3/velocity/4.1.0-SNAPSHOT/21/download":
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

func TestVelocityVersionLess(t *testing.T) {
	cases := []struct {
		a, b string
		less bool
	}{
		{"3.5.1", "4.0.0", true},
		{"4.0.0", "3.5.1", false},
		{"4.1.0-SNAPSHOT", "4.0.0", false},
		{"4.0.0", "4.1.0-SNAPSHOT", true},
		{"4.1.0-SNAPSHOT", "4.1.0", true},
		{"4.1.0", "4.1.0-SNAPSHOT", false},
		{"4.1.0-SNAPSHOT", "4.1.0-SNAPSHOT", false},
		{"3.6.0-SNAPSHOT", "3.5.1", false},
		{"3.5.1", "3.6.0-SNAPSHOT", true},
		{"3.5.1", "3.5.1", false},
	}
	for _, c := range cases {
		if got := velocityVersionLess(c.a, c.b); got != c.less {
			t.Errorf("velocityVersionLess(%q, %q) = %v, want %v", c.a, c.b, got, c.less)
		}
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
	// newest version line wins (4.1.0-SNAPSHOT), best build within it (21).
	// the RECOMMENDED 3.5.1 tag and the EXPERIMENTAL build 999 must not win.
	if best.version != "4.1.0-SNAPSHOT" || best.build.ID != 21 {
		t.Errorf("expected 4.1.0-SNAPSHOT build 21, got %s build %d (channel %s)", best.version, best.build.ID, best.build.Channel)
	}
}

func TestCheckVelocityUpdate(t *testing.T) {
	velocityUpdateCacheMu.Lock()
	velocityUpdateCache = make(map[string]*cachedUpdate)
	velocityUpdateCacheMu.Unlock()

	defer withVelocityMock(t)()

	// old major -> update available
	info, err := CheckVelocityUpdate("3.5.0-SNAPSHOT", 584)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if !info.HasUpdate || info.LatestVersion != "4.1.0-SNAPSHOT" || info.LatestBuild != 21 {
		t.Errorf("expected update to 4.1.0-SNAPSHOT/21, got %+v", info)
	}

	// recommended tag on old major -> still update available
	info, err = CheckVelocityUpdate("3.5.1", 615)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if !info.HasUpdate || info.LatestVersion != "4.1.0-SNAPSHOT" {
		t.Errorf("expected update from 3.5.1 to 4.1.0-SNAPSHOT, got %+v", info)
	}

	// same version, older build -> update available
	info, err = CheckVelocityUpdate("4.1.0-SNAPSHOT", 19)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if !info.HasUpdate || info.LatestBuild != 21 {
		t.Errorf("expected build-only update to 21, got %+v", info)
	}

	// fully current -> no update
	info, err = CheckVelocityUpdate("4.1.0-SNAPSHOT", 21)
	if err != nil {
		t.Fatalf("CheckVelocityUpdate() error = %v", err)
	}
	if info.HasUpdate {
		t.Errorf("expected no update when on 4.1.0-SNAPSHOT/21, got %+v", info)
	}
}

func TestDownloadVelocityJarPicksSameBuild(t *testing.T) {
	defer withVelocityMock(t)()

	dest := filepath.Join(t.TempDir(), "velocity.jar")
	version, build, err := DownloadVelocityJar(dest)
	if err != nil {
		t.Fatalf("DownloadVelocityJar() error = %v", err)
	}

	if version != "4.1.0-SNAPSHOT" || build != 21 {
		t.Errorf("expected download of 4.1.0-SNAPSHOT/21, got %s/%d", version, build)
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
