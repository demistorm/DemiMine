package minimotd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeProxyIcon(t *testing.T, proxiesDir, proxyName string) {
	t.Helper()
	iconsDir := filepath.Join(proxiesDir, proxyName, "plugins/minimotd-velocity", "icons")
	if err := os.MkdirAll(iconsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iconsDir, proxyName+".png"), []byte("png"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateExtraConfigOwnIcon(t *testing.T) {
	m := NewManager(t.TempDir())
	out := m.generateExtraConfig("myproxy", "myserver", "line1", "line2", true)
	if !strings.Contains(out, "icon=myserver-server") {
		t.Errorf("expected own icon line, got:\n%s", out)
	}
}

func TestGenerateExtraConfigFallsBackToProxyIcon(t *testing.T) {
	dir := t.TempDir()
	writeProxyIcon(t, dir, "myproxy")
	m := NewManager(dir)
	out := m.generateExtraConfig("myproxy", "myserver", "line1", "line2", false)
	if !strings.Contains(out, "icon=myproxy\n") {
		t.Errorf("expected proxy icon fallback, got:\n%s", out)
	}
}

func TestGenerateExtraConfigNoIconAnywhere(t *testing.T) {
	m := NewManager(t.TempDir())
	out := m.generateExtraConfig("myproxy", "myserver", "line1", "line2", false)
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "icon=") {
			t.Errorf("expected no icon line without proxy icon, got %q", trimmed)
		}
	}
}

func TestSyncServerConfigSkipsServersWithoutExtraConfig(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	serverPath := filepath.Join(dir, "myserver")
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		t.Fatal(err)
	}

	// no extra config exists — sync should be a no-op, not create one
	m.SyncServerConfig("myproxy", "myserver", serverPath, "l1", "l2")

	if _, err := os.Stat(m.getExtraConfigPath("myproxy", "myserver")); !os.IsNotExist(err) {
		t.Error("expected no extra config to be created")
	}
}

func TestSyncServerConfigHealsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	writeProxyIcon(t, dir, "myproxy")
	m := NewManager(dir)
	serverPath := filepath.Join(dir, "myserver")
	if err := os.MkdirAll(serverPath, 0755); err != nil {
		t.Fatal(err)
	}
	if err := m.CreateExtraConfig("myproxy", "myserver", "l1", "l2", false); err != nil {
		t.Fatal(err)
	}

	// old config had no icon line; sync should add the proxy fallback
	m.SyncServerConfig("myproxy", "myserver", serverPath, "l1", "l2")

	data, err := os.ReadFile(m.getExtraConfigPath("myproxy", "myserver"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "icon=myproxy\n") {
		t.Errorf("expected healed config with proxy icon, got:\n%s", data)
	}
}
