package origin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissing(t *testing.T) {
	m, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Errorf("want empty map, got %v", m)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	in := map[string]Origin{
		"scoped-commits":  {Repo: "https://github.com/a/b", Path: "skills/scoped-commits", Hash: "abc"},
		"frontend-design": {Repo: "https://github.com/anthropics/skills", Path: "frontend-design", Hash: "def"},
	}
	if err := Save(dir, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out["scoped-commits"] != in["scoped-commits"] || out["frontend-design"] != in["frontend-design"] {
		t.Errorf("round trip mismatch: %v", out)
	}
}

func TestSaveEmptyRemoves(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, map[string]Origin{"x": {Repo: "r"}}); err != nil {
		t.Fatal(err)
	}
	if err := Save(dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(File(dir)); !os.IsNotExist(err) {
		t.Error("empty save should remove the manifest")
	}
	if err := Save(dir, nil); err != nil {
		t.Errorf("empty save without a manifest should be a no-op, got %v", err)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHashDir(t *testing.T) {
	a := t.TempDir()
	write(t, a, "SKILL.md", "hello")
	write(t, a, "ref/notes.md", "world")

	b := t.TempDir()
	write(t, b, "SKILL.md", "hello")
	write(t, b, "ref/notes.md", "world")
	write(t, b, ".DS_Store", "junk")

	ha, err := HashDir(a)
	if err != nil {
		t.Fatal(err)
	}
	hb, err := HashDir(b)
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Error("identical content (modulo .DS_Store) should hash equal")
	}

	write(t, b, "ref/notes.md", "world!")
	hb2, err := HashDir(b)
	if err != nil {
		t.Fatal(err)
	}
	if hb2 == hb {
		t.Error("changed content should change the hash")
	}
}
