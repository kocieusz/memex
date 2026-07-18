package target

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kocieusz/memex/internal/library"
)

// fixture builds a library with the given skills and an empty target dir.
func fixture(t *testing.T, skillNames ...string) (libDir, targetDir string, skills []library.Skill) {
	t.Helper()
	root := t.TempDir()
	libDir = filepath.Join(root, "library")
	targetDir = filepath.Join(root, "target")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range skillNames {
		mkSkill(t, filepath.Join(libDir, name))
	}
	var err error
	skills, err = library.Scan(libDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != len(skillNames) {
		t.Fatalf("library scan found %d skills, want %d", len(skills), len(skillNames))
	}
	return libDir, targetDir, skills
}

func mkSkill(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: x\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func stateOf(t *testing.T, entries []Entry, name string) Entry {
	t.Helper()
	for _, e := range entries {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no entry named %q in %v", name, entries)
	return Entry{}
}

func TestScanClassifiesEveryState(t *testing.T) {
	libDir, targetDir, skills := fixture(t, "alpha", "beta")

	// alpha linked, beta left available
	if err := Link(targetDir, skills[0]); err != nil {
		t.Fatal(err)
	}
	// a native dir owned by the target
	mkSkill(t, filepath.Join(targetDir, "native-skill"))
	// a foreign symlink
	outside := filepath.Join(t.TempDir(), "elsewhere")
	mkSkill(t, outside)
	if err := os.Symlink(outside, filepath.Join(targetDir, "foreign-skill")); err != nil {
		t.Fatal(err)
	}
	// a broken symlink
	if err := os.Symlink(filepath.Join(libDir, "gone"), filepath.Join(targetDir, "dead")); err != nil {
		t.Fatal(err)
	}
	// junk that must be invisible
	if err := os.WriteFile(filepath.Join(targetDir, ".DS_Store"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(targetDir, ".system"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "notes.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := Scan(targetDir, skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("got %d entries, want 5: %+v", len(entries), entries)
	}
	for name, want := range map[string]State{
		"alpha":         Linked,
		"beta":          Available,
		"native-skill":  Native,
		"foreign-skill": Foreign,
		"dead":          Broken,
	} {
		if got := stateOf(t, entries, name).State; got != want {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
}

func TestScanMissingTargetDirIsEmpty(t *testing.T) {
	_, targetDir, skills := fixture(t, "alpha")
	entries, err := Scan(filepath.Join(targetDir, "does-not-exist"), skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].State != Available {
		t.Fatalf("want single available entry, got %+v", entries)
	}
}

func TestScanNativeShadowsLibrarySkill(t *testing.T) {
	_, targetDir, skills := fixture(t, "alpha")
	mkSkill(t, filepath.Join(targetDir, "alpha"))

	entries, err := Scan(targetDir, skills)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("collision must yield one entry, got %+v", entries)
	}
	e := entries[0]
	if e.State != Native || !e.Shadows {
		t.Fatalf("want shadowing native, got %+v", e)
	}
}

func TestLinkIsIdempotent(t *testing.T) {
	_, targetDir, skills := fixture(t, "alpha")
	if err := Link(targetDir, skills[0]); err != nil {
		t.Fatal(err)
	}
	if err := Link(targetDir, skills[0]); err != nil {
		t.Fatalf("second link must be a no-op, got %v", err)
	}
}

func TestLinkRefusesExistingNative(t *testing.T) {
	_, targetDir, skills := fixture(t, "alpha")
	mkSkill(t, filepath.Join(targetDir, "alpha"))
	if err := Link(targetDir, skills[0]); err == nil {
		t.Fatal("linking over a native dir must fail")
	}
}

func TestLinkCreatesMissingTargetDir(t *testing.T) {
	_, targetDir, skills := fixture(t, "alpha")
	fresh := filepath.Join(targetDir, "nested", "skills")
	if err := Link(fresh, skills[0]); err != nil {
		t.Fatal(err)
	}
	entries, err := Scan(fresh, skills)
	if err != nil {
		t.Fatal(err)
	}
	if stateOf(t, entries, "alpha").State != Linked {
		t.Fatal("skill not linked in fresh dir")
	}
}

func TestUnlinkRemovesLibraryLink(t *testing.T) {
	libDir, targetDir, skills := fixture(t, "alpha")
	if err := Link(targetDir, skills[0]); err != nil {
		t.Fatal(err)
	}
	if err := Unlink(targetDir, libDir, "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(targetDir, "alpha")); !os.IsNotExist(err) {
		t.Fatal("link still present after unlink")
	}
}

// The safety invariant: Unlink must refuse anything that is not a symlink into
// the library.
func TestUnlinkRefusesNativeDir(t *testing.T) {
	libDir, targetDir, _ := fixture(t, "alpha")
	mkSkill(t, filepath.Join(targetDir, "precious"))
	if err := Unlink(targetDir, libDir, "precious"); err == nil {
		t.Fatal("unlink of a real directory must fail")
	}
	if _, err := os.Stat(filepath.Join(targetDir, "precious", "SKILL.md")); err != nil {
		t.Fatal("native dir was damaged")
	}
}

func TestUnlinkRefusesForeignSymlink(t *testing.T) {
	libDir, targetDir, _ := fixture(t, "alpha")
	outside := filepath.Join(t.TempDir(), "elsewhere")
	mkSkill(t, outside)
	if err := os.Symlink(outside, filepath.Join(targetDir, "foreign")); err != nil {
		t.Fatal(err)
	}
	if err := Unlink(targetDir, libDir, "foreign"); err == nil {
		t.Fatal("unlink of a foreign symlink must fail")
	}
	if _, err := os.Lstat(filepath.Join(targetDir, "foreign")); err != nil {
		t.Fatal("foreign symlink was removed")
	}
}

func TestUnlinkRemovesRelativeLibraryLink(t *testing.T) {
	libDir, targetDir, _ := fixture(t, "alpha")
	rel, err := filepath.Rel(targetDir, filepath.Join(libDir, "alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(rel, filepath.Join(targetDir, "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := Unlink(targetDir, libDir, "alpha"); err != nil {
		t.Fatalf("relative link into library must be removable: %v", err)
	}
}
