package config

import (
	"os"
	"path/filepath"
	"slices"
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

func TestLoadDisabledProviders(t *testing.T) {
	write(t, "library = \"/tmp/skills\"\ndisabled_providers = [\"pi\", \"agents\"]\n")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"pi", "agents"}; !slices.Equal(c.DisabledProviders, want) {
		t.Errorf("DisabledProviders = %v, want %v", c.DisabledProviders, want)
	}
}

// read returns the config file's contents, for the writer tests.
func read(t *testing.T) string {
	t.Helper()
	path, err := Path()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// The config file is hand-edited, so writing one key must not disturb the rest.
func TestSetDisabledProvidersPreservesTheRestOfTheFile(t *testing.T) {
	write(t, "# my library lives in dotfiles\nlibrary = \"~/.dotfiles/skills\"\n")
	if err := SetDisabledProviders([]string{"pi"}); err != nil {
		t.Fatal(err)
	}
	got := read(t)
	want := "# my library lives in dotfiles\nlibrary = \"~/.dotfiles/skills\"\ndisabled_providers = [\"pi\"]\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.Library != "~/.dotfiles/skills" || !slices.Equal(c.DisabledProviders, []string{"pi"}) {
		t.Errorf("round trip lost settings: %+v", c)
	}
}

func TestSetDisabledProvidersReplacesExistingKey(t *testing.T) {
	write(t, "disabled_providers = [\"pi\"]\nlibrary = \"/l\"\n")
	if err := SetDisabledProviders([]string{"agents", "codex"}); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t), "disabled_providers = [\"agents\", \"codex\"]\nlibrary = \"/l\"\n"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A hand-written array can span lines; all of it has to go.
func TestSetDisabledProvidersReplacesMultiLineArray(t *testing.T) {
	write(t, "library = \"/l\"\ndisabled_providers = [\n  \"pi\",\n  \"agents\",\n]\n# trailing comment\n")
	if err := SetDisabledProviders([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t), "library = \"/l\"\ndisabled_providers = [\"codex\"]\n# trailing comment\n"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetDisabledProvidersEmptyRemovesKey(t *testing.T) {
	write(t, "library = \"/l\"\ndisabled_providers = [\"pi\"]\n")
	if err := SetDisabledProviders(nil); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t), "library = \"/l\"\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// The key is top-level, so it has to go above any table — below one it would
// belong to that table.
func TestSetDisabledProvidersInsertsAboveTables(t *testing.T) {
	write(t, "library = \"/l\"\n\n[something]\nkey = 1\n")
	if err := SetDisabledProviders([]string{"pi"}); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t), "library = \"/l\"\n\ndisabled_providers = [\"pi\"]\n[something]\nkey = 1\n"; got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestSetDisabledProvidersCreatesConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := SetDisabledProviders([]string{"pi"}); err != nil {
		t.Fatal(err)
	}
	if got, want := read(t), "disabled_providers = [\"pi\"]\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
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
