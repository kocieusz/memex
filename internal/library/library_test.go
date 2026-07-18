package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanFindsOnlySkillDirs(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"beta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "SKILL.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// not skills: no SKILL.md, dot-dir, plain file
	if err := os.MkdirAll(filepath.Join(dir, "not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	skills, err := Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 2 || skills[0].Name != "alpha" || skills[1].Name != "beta" {
		t.Fatalf("want sorted [alpha beta], got %+v", skills)
	}
}

func TestScaffold(t *testing.T) {
	dir := t.TempDir()
	if _, err := Scaffold(dir, "my-skill"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "my-skill", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(content) == 0 {
		t.Fatal("empty SKILL.md")
	}
	if _, err := Scaffold(dir, "my-skill"); err == nil {
		t.Fatal("scaffolding an existing skill must fail")
	}
	for _, bad := range []string{"My-Skill", "has space", "-lead", "trail-", ""} {
		if _, err := Scaffold(dir, bad); err == nil {
			t.Fatalf("name %q must be rejected", bad)
		}
	}
}
