package mc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultServerProperties(t *testing.T) {
	props := DefaultServerProperties()

	if props["server-port"] != "25565" {
		t.Errorf("Expected server-port=25565, got %s", props["server-port"])
	}

	if props["online-mode"] != "false" {
		t.Errorf("Expected online-mode=false, got %s", props["online-mode"])
	}

	if props["motd"] != "A DemiMine Server" {
		t.Errorf("Expected default motd, got %s", props["motd"])
	}

	if props["allow-flight"] != "true" {
		t.Errorf("Expected allow-flight=true, got %s", props["allow-flight"])
	}
}

func TestServerPropertiesSetGet(t *testing.T) {
	props := DefaultServerProperties()

	props.Set("server-port", "25566")
	if props.Get("server-port") != "25566" {
		t.Errorf("Expected server-port=25566 after Set, got %s", props.Get("server-port"))
	}

	props.Set("custom-key", "custom-value")
	if props.Get("custom-key") != "custom-value" {
		t.Errorf("Expected custom-key=custom-value, got %s", props.Get("custom-key"))
	}
}

func TestServerPropertiesWriteRead(t *testing.T) {
	props := DefaultServerProperties()
	props.Set("server-port", "25567")
	props.Set("motd", "Test Server")
	props.Set("custom-prop", "test-value")

	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "server.properties")

	if err := props.WriteToFile(tmpFile); err != nil {
		t.Fatalf("WriteToFile error: %v", err)
	}

	readProps, err := ReadServerProperties(tmpFile)
	if err != nil {
		t.Fatalf("ReadServerProperties error: %v", err)
	}

	if readProps["server-port"] != "25567" {
		t.Errorf("Expected server-port=25567, got %s", readProps["server-port"])
	}

	if readProps["motd"] != "Test Server" {
		t.Errorf("Expected motd='Test Server', got %s", readProps["motd"])
	}

	if readProps["custom-prop"] != "test-value" {
		t.Errorf("Expected custom-prop=test-value, got %s", readProps["custom-prop"])
	}
}

func TestServerPropertiesWriteSorted(t *testing.T) {
	props := make(ServerProperties)
	props["zebra"] = "a"
	props["alpha"] = "b"
	props["beta"] = "c"

	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "server.properties")

	if err := props.WriteToFile(tmpFile); err != nil {
		t.Fatalf("WriteToFile error: %v", err)
	}

	data, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		t.Fatalf("Expected at least 3 lines, got %d", len(lines))
	}

	if lines[0] != "alpha=b" {
		t.Errorf("Expected first line to be 'alpha=b' (sorted), got %s", lines[0])
	}
}

func TestReadServerPropertiesWithComments(t *testing.T) {
	content := `# This is a comment
server-port=25565
# Another comment
motd=Test Server
`
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "server.properties")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	props, err := ReadServerProperties(tmpFile)
	if err != nil {
		t.Fatalf("ReadServerProperties error: %v", err)
	}

	if len(props) != 2 {
		t.Errorf("Expected 2 properties (ignoring comments), got %d", len(props))
	}

	if props["server-port"] != "25565" {
		t.Errorf("Expected server-port=25565, got %s", props["server-port"])
	}

	if props["motd"] != "Test Server" {
		t.Errorf("Expected motd='Test Server', got %s", props["motd"])
	}
}

func TestMergeServerProperties(t *testing.T) {
	base := ServerProperties{
		"server-port": "25565",
		"motd":          "Base Server",
		"online-mode":  "true",
	}

	override := ServerProperties{
		"server-port": "25566",
		"motd":          "Override Server",
	}

	merged := MergeServerProperties(base, override)

	if merged["server-port"] != "25566" {
		t.Errorf("Expected server-port=25566 (overridden), got %s", merged["server-port"])
	}

	if merged["motd"] != "Override Server" {
		t.Errorf("Expected motd='Override Server' (overridden), got %s", merged["motd"])
	}

	if merged["online-mode"] != "true" {
		t.Errorf("Expected online-mode=true (from base), got %s", merged["online-mode"])
	}
}

func TestReadServerPropertiesEmpty(t *testing.T) {
	content := `# Only comments
# No properties
`
	tmpDir := t.TempDir()
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, "server.properties")
	if err := os.WriteFile(tmpFile, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}

	props, err := ReadServerProperties(tmpFile)
	if err != nil {
		t.Fatalf("ReadServerProperties error: %v", err)
	}

	if len(props) != 0 {
		t.Errorf("Expected 0 properties, got %d", len(props))
	}
}
