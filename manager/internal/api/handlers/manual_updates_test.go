package handlers

import (
	"bytes"
	"context"
	"database/sql"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/demimine/manager/internal/config"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

func TestManualJarFilename(t *testing.T) {
	cases := []struct {
		serverType string
		want       string
		ok         bool
	}{
		{"paper", "server.jar", true},
		{"purpur", "server.jar", true},
		{"nanolimbo", "server.jar", true},
		{"fabric", "fabric-server-launch.jar", true},
		{"neoforge", "neoforge-installer.jar", true},
		{"forge", "forge-installer.jar", true},
		{"velocity", "", false},
		{"bungee", "", false},
		{"", "", false},
	}
	for _, c := range cases {
		got, ok := manualJarFilename(c.serverType)
		if ok != c.ok || got != c.want {
			t.Errorf("manualJarFilename(%q) = (%q, %v), want (%q, %v)", c.serverType, got, ok, c.want, c.ok)
		}
	}
}

func TestManualUpdateCleanupPaths(t *testing.T) {
	if got := manualUpdateCleanupPaths("fabric"); len(got) != 3 {
		t.Errorf("fabric cleanup should remove stub state, .fabric and vanilla server.jar, got %v", got)
	}
	for _, p := range manualUpdateCleanupPaths("fabric") {
		if p == "" {
			t.Error("fabric cleanup contains empty path")
		}
	}
	if got := manualUpdateCleanupPaths("neoforge"); len(got) != 1 || got[0] != "run.sh" {
		t.Errorf("neoforge cleanup = %v, want [run.sh]", got)
	}
	if got := manualUpdateCleanupPaths("forge"); len(got) != 1 || got[0] != "run.sh" {
		t.Errorf("forge cleanup = %v, want [run.sh]", got)
	}
	if got := manualUpdateCleanupPaths("paper"); got != nil {
		t.Errorf("paper cleanup should be nil, got %v", got)
	}
}

// fakeJar builds a minimal byte slice that passes the zip magic check
func fakeJar(content string) []byte {
	return append([]byte("PK\x03\x04"), []byte(content)...)
}

func newManualUpdateTestEnv(t *testing.T) (*JarUpdateHandler, *httptest.ResponseRecorder, func()) {
	t.Helper()

	serversDir := t.TempDir()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE servers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		type TEXT NOT NULL,
		version TEXT NOT NULL,
		sanitized_name TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'stopped',
		jar_build INTEGER DEFAULT 0,
		jar_hash TEXT DEFAULT '',
		updated_at DATETIME
	)`); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{ServersDir: serversDir}
	h := NewJarUpdateHandler(db, cfg)

	cleanup := func() { db.Close() }
	return h, nil, cleanup
}

func doManualUpdate(h *JarUpdateHandler, serverID, serverType, status, version, build string, jarBytes []byte) *httptest.ResponseRecorder {
	db := h.db
	if serverType != "" {
		db.Exec("INSERT INTO servers (id, type, version, sanitized_name, status) VALUES (?, ?, '1.0.0', 'testsrv', ?)", serverID, serverType, status)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "custom.jar")
	fw.Write(jarBytes)
	mw.WriteField("version", version)
	if build != "" {
		mw.WriteField("build", build)
	}
	mw.Close()

	req := httptest.NewRequest("POST", "/api/servers/{id}/jar-update/manual", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", serverID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	w := httptest.NewRecorder()
	h.ManualServerJarUpdate(w, req)
	return w
}

func TestManualUpdatePaperSetsVersionAndBuild(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "paper", "stopped", "26.2", "42", fakeJar("paper-jar"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var version string
	var jarBuild int
	var jarHash string
	h.db.QueryRow("SELECT version, jar_build, jar_hash FROM servers WHERE id = 1").Scan(&version, &jarBuild, &jarHash)
	if version != "26.2" || jarBuild != 42 || jarHash != "" {
		t.Errorf("db = (%q, %d, %q), want (26.2, 42, '')", version, jarBuild, jarHash)
	}

	data, err := os.ReadFile(filepath.Join(h.cfg.ServersDir, "testsrv", "server.jar"))
	if err != nil {
		t.Fatalf("server.jar not written: %v", err)
	}
	if !bytes.HasPrefix(data, []byte("PK\x03\x04")) || !strings.Contains(string(data), "paper-jar") {
		t.Error("server.jar content mismatch")
	}
}

func TestManualUpdateNanolimboKeepsVersionOnly(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "nanolimbo", "stopped", "1.13.1", "", fakeJar("limbo"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var version string
	h.db.QueryRow("SELECT version FROM servers WHERE id = 1").Scan(&version)
	if version != "1.13.1" {
		t.Errorf("version = %q, want 1.13.1", version)
	}
	if _, err := os.Stat(filepath.Join(h.cfg.ServersDir, "testsrv", "server.jar")); err != nil {
		t.Fatalf("server.jar not written: %v", err)
	}
}

func TestManualUpdateFabricClearsStubState(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	srvPath := filepath.Join(h.cfg.ServersDir, "testsrv")
	os.MkdirAll(filepath.Join(srvPath, ".fabric", "server"), 0755)
	os.WriteFile(filepath.Join(srvPath, "server.jar"), []byte("vanilla"), 0644)
	os.WriteFile(filepath.Join(srvPath, "fabric-server-launcher.properties"), []byte("serverJar=server.jar"), 0644)
	os.WriteFile(filepath.Join(srvPath, "fabric-server-launch.jar"), []byte("old stub"), 0644)

	w := doManualUpdate(h, "1", "fabric", "stopped", "26.2", "", fakeJar("standalone-launcher"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if data, err := os.ReadFile(filepath.Join(srvPath, "fabric-server-launch.jar")); err != nil || !strings.Contains(string(data), "standalone-launcher") {
		t.Error("fabric-server-launch.jar should contain the uploaded launcher")
	}
	for _, gone := range []string{"server.jar", "fabric-server-launcher.properties", ".fabric"} {
		if _, err := os.Stat(filepath.Join(srvPath, gone)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed, stat err = %v", gone, err)
		}
	}
}

func TestManualUpdateNeoForgeRemovesRunSh(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	srvPath := filepath.Join(h.cfg.ServersDir, "testsrv")
	os.MkdirAll(filepath.Join(srvPath, "libraries"), 0755)
	os.WriteFile(filepath.Join(srvPath, "run.sh"), []byte("#!/bin/sh"), 0755)

	w := doManualUpdate(h, "1", "neoforge", "stopped", "26.2", "", fakeJar("installer"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := os.Stat(filepath.Join(srvPath, "neoforge-installer.jar")); err != nil {
		t.Fatalf("neoforge-installer.jar not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srvPath, "run.sh")); !os.IsNotExist(err) {
		t.Error("run.sh should have been removed so the installer re-runs")
	}
	if _, err := os.Stat(filepath.Join(srvPath, "libraries")); err != nil {
		t.Error("libraries/ should be left in place for the installer to update")
	}
}

func TestManualUpdateForgeRemovesRunSh(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	srvPath := filepath.Join(h.cfg.ServersDir, "testsrv")
	os.WriteFile(filepath.Join(srvPath, "run.sh"), []byte("#!/bin/sh"), 0755)

	w := doManualUpdate(h, "1", "forge", "stopped", "1.21.1", "", fakeJar("installer"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	if _, err := os.Stat(filepath.Join(srvPath, "forge-installer.jar")); err != nil {
		t.Fatalf("forge-installer.jar not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(srvPath, "run.sh")); !os.IsNotExist(err) {
		t.Error("run.sh should have been removed")
	}
}

func TestManualUpdateRunningServerRejected(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "paper", "running", "26.2", "", fakeJar("jar"))
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestManualUpdateRejectsNonJar(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "paper", "stopped", "26.2", "", []byte("definitely not a jar"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	// no stray temp file left behind
	if _, err := os.Stat(filepath.Join(h.cfg.ServersDir, "testsrv", ".upload_temp.jar")); !os.IsNotExist(err) {
		t.Error("temp file should be cleaned up on failure")
	}
}

func TestManualUpdateRejectsMissingVersion(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "paper", "stopped", "   ", "", fakeJar("jar"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for blank version, got %d", w.Code)
	}
}

func TestManualUpdateRejectsPathTraversalVersion(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "paper", "stopped", "../../etc", "", fakeJar("jar"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for traversal version, got %d", w.Code)
	}
}

func TestManualUpdateRejectsInvalidBuild(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "1", "paper", "stopped", "26.2", "not-a-number", fakeJar("jar"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad build, got %d", w.Code)
	}

	w = doManualUpdate(h, "1", "purpur", "stopped", "26.2", "-5", fakeJar("jar"))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for negative build, got %d", w.Code)
	}
}

func TestManualUpdateUnknownServer(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	w := doManualUpdate(h, "99", "", "", "26.2", "", fakeJar("jar"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}}

func TestManualUpdateLargeFileStreamsToDisk(t *testing.T) {
	h, _, cleanup := newManualUpdateTestEnv(t)
	defer cleanup()

	// ~2MB payload to confirm multipart temp handling works through the handler
	payload := append([]byte("PK\x03\x04"), bytes.Repeat([]byte{0xAB}, 2<<20)...)
	w := doManualUpdate(h, "1", "paper", "stopped", "26.2", "", payload)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	info, err := os.Stat(filepath.Join(h.cfg.ServersDir, "testsrv", "server.jar"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != int64(len(payload)) {
		t.Errorf("jar size = %d, want %d", info.Size(), len(payload))
	}
}
