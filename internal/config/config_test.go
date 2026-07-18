package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".memex"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".memex", "config.toml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingFile(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	c, err := Load()
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if c.Library != "" {
		t.Errorf("Library = %q, want empty", c.Library)
	}
}

func TestLoadLibrary(t *testing.T) {
	write(t, `library = "~/.dotfiles/skills"`+"\n")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Library != "~/.dotfiles/skills" {
		t.Errorf("Library = %q, want ~/.dotfiles/skills", c.Library)
	}
}

func TestLoadMalformed(t *testing.T) {
	write(t, `library = `)
	if _, err := Load(); err == nil {
		t.Fatal("malformed file should error")
	}
}

func TestLoadUnknownKey(t *testing.T) {
	write(t, `libary = "/tmp/skills"`+"\n")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "unknown key") {
		t.Fatalf("unknown key should error, got %v", err)
	}
}

func TestPath(t *testing.T) {
	t.Setenv("HOME", "/custom")
	p, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	if p != "/custom/.memex/config.toml" {
		t.Errorf("Path = %q", p)
	}
}
