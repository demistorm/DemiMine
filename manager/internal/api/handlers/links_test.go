package handlers

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeLinkPath(t *testing.T) {
	cases := []struct {
		in   string
		out  string
		ok   bool
	}{
		{"/plugins/LuckPerms/", "plugins/LuckPerms", true},
		{"plugins/LuckPerms", "plugins/LuckPerms", true},
		{"/server.properties", "server.properties", true},
		{"//a//b//", "a/b", true},
		{"plugins/../server.properties", "server.properties", true}, // cleans back inside the tree, fine
		{"", "", false},
		{"/", "", false},
		{"..", "", false},
		{"   ", "", false},
	}
	for _, c := range cases {
		got, ok := normalizeLinkPath(c.in)
		if ok != c.ok || got != c.out {
			t.Errorf("normalizeLinkPath(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.out, c.ok)
		}
	}
}

func TestPathIsUnder(t *testing.T) {
	if !pathIsUnder("plugins", "plugins") {
		t.Error("exact match should count")
	}
	if !pathIsUnder("plugins", "plugins/LuckPerms") {
		t.Error("child should count")
	}
	if pathIsUnder("plugins", "plugins-extra") {
		t.Error("sibling with shared prefix should not count")
	}
	if pathIsUnder("plugins/LuckPerms", "plugins") {
		t.Error("parent should not count")
	}
}

func TestCopyPathFileAndDir(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// file copy
	if err := os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyPath(filepath.Join(src, "a.txt"), filepath.Join(dst, "a.txt")); err != nil {
		t.Fatalf("file copy failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dst, "a.txt"))
	if err != nil || string(data) != "hello" {
		t.Fatalf("file copy content mismatch: %q %v", data, err)
	}

	// dir copy with nesting
	nested := filepath.Join(src, "tree", "inner")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "b.txt"), []byte("deep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := copyPath(filepath.Join(src, "tree"), filepath.Join(dst, "tree")); err != nil {
		t.Fatalf("dir copy failed: %v", err)
	}
	data, err = os.ReadFile(filepath.Join(dst, "tree", "inner", "b.txt"))
	if err != nil || string(data) != "deep" {
		t.Fatalf("dir copy content mismatch: %q %v", data, err)
	}
}
