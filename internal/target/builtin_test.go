package target

import (
	"os"
	"path/filepath"
	"testing"
)

// Shells don't expand ~ inside --flag=~/... values, so Resolve must.
func TestResolveExpandsTilde(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for in, want := range map[string]string{
		"~/.claude/skills":  filepath.Join(home, ".claude", "skills"),
		"~/.claude/skills/": filepath.Join(home, ".claude", "skills"),
		"~":                 home,
	} {
		got, err := Resolve(in)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("Resolve(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveRejectsUnknownName(t *testing.T) {
	if _, err := Resolve("no-such-harness"); err == nil {
		t.Fatal("bare unknown name must not resolve to a path")
	}
}
