package remote

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRef(t *testing.T) {
	cases := []struct {
		arg  string
		want Ref
	}{
		{"anthropics/skills", Ref{URL: "https://github.com/anthropics/skills", Name: "skills"}},
		{"https://github.com/anthropics/skills", Ref{URL: "https://github.com/anthropics/skills", Name: "skills"}},
		{"https://github.com/anthropics/skills.git", Ref{URL: "https://github.com/anthropics/skills.git", Name: "skills"}},
		{"git@github.com:anthropics/skills.git", Ref{URL: "git@github.com:anthropics/skills.git", Name: "skills"}},
		{"https://github.com/anthropics/skills/tree/dev", Ref{URL: "https://github.com/anthropics/skills", Name: "skills", Branch: "dev"}},
		{"https://github.com/anthropics/skills/tree/dev/document-skills/pdf", Ref{URL: "https://github.com/anthropics/skills", Name: "skills", Branch: "dev", Subdir: "document-skills/pdf"}},
	}
	for _, c := range cases {
		got, err := ParseRef(c.arg)
		if err != nil {
			t.Fatalf("ParseRef(%q): %v", c.arg, err)
		}
		if got != c.want {
			t.Fatalf("ParseRef(%q) = %+v, want %+v", c.arg, got, c.want)
		}
	}
}

func TestParseRefRejectsGarbage(t *testing.T) {
	for _, arg := range []string{"", "just-a-name", "./local/path", "https://github.com/onlyowner"} {
		if _, err := ParseRef(arg); err == nil {
			t.Fatalf("ParseRef(%q) must fail", arg)
		}
	}
}

// makeSkill creates dir with a SKILL.md carrying desc in its frontmatter.
func makeSkill(t *testing.T, dir, desc string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: x\ndescription: " + desc + "\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverFindsNestedSkillsAndSkipsDotDirs(t *testing.T) {
	root := t.TempDir()
	makeSkill(t, filepath.Join(root, "alpha"), "first")
	makeSkill(t, filepath.Join(root, "group", "beta"), "second")
	makeSkill(t, filepath.Join(root, ".git", "hidden"), "never")
	// a nested SKILL.md inside a skill must not become its own entry
	makeSkill(t, filepath.Join(root, "alpha", "inner"), "never")

	got, err := Discover(root, Ref{Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Description != "first" || got[1].Rel != "group/beta" {
		t.Fatalf("got %+v", got)
	}
}

func TestDiscoverFoldsBlockScalarDescription(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "alpha")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: alpha\ndescription: >-\n  first line\n  second line\n\n  third line\nlicense: MIT\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Discover(root, Ref{Name: "repo"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "first line second line third line"; len(got) != 1 || got[0].Description != want {
		t.Fatalf("got %+v, want description %q", got, want)
	}
}

func TestDiscoverRepoRootSkillUsesRefName(t *testing.T) {
	root := t.TempDir()
	makeSkill(t, root, "root skill")
	got, err := Discover(root, Ref{Name: "myskill"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "myskill" || got[0].Rel != "." {
		t.Fatalf("got %+v", got)
	}
}

func TestDiscoverHonorsSubdir(t *testing.T) {
	root := t.TempDir()
	makeSkill(t, filepath.Join(root, "alpha"), "outside")
	makeSkill(t, filepath.Join(root, "sub", "beta"), "inside")

	got, err := Discover(root, Ref{Name: "repo", Subdir: "sub"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Name != "beta" {
		t.Fatalf("got %+v", got)
	}
	if _, err := Discover(root, Ref{Name: "repo", Subdir: "missing"}); err == nil {
		t.Fatal("missing subdir must fail")
	}
}

func TestCopyCopiesTreeAndRefusesOverwrite(t *testing.T) {
	root, lib := t.TempDir(), t.TempDir()
	dir := filepath.Join(root, "alpha")
	makeSkill(t, dir, "d")
	if err := os.MkdirAll(filepath.Join(dir, "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "refs", "extra.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := Skill{Name: "alpha", Path: dir}
	if err := Copy(s, lib); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"SKILL.md", filepath.Join("refs", "extra.md")} {
		if _, err := os.Stat(filepath.Join(lib, "alpha", f)); err != nil {
			t.Fatalf("missing %s: %v", f, err)
		}
	}
	if err := Copy(s, lib); err == nil {
		t.Fatal("second copy must refuse to overwrite")
	}
}
