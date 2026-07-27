package project

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadMissingManifest(t *testing.T) {
	s, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("a library without a manifest must not error: %v", err)
	}
	if s.Found {
		t.Error("Found must be false so callers can seed the default set")
	}
	if len(s.Skills(DefaultName)) != 0 || len(s.Names()) != 0 {
		t.Errorf("want an empty store, got %+v", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	lib := t.TempDir()
	var s Store
	s.SetSkills(DefaultName, []string{"scoped-commits"})
	s.SetSkills("web", []string{"dataviz", "tanstack-router"})
	if err := Save(lib, s); err != nil {
		t.Fatal(err)
	}

	got, err := Load(lib)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Found {
		t.Error("Found must be true once the manifest exists")
	}
	if want := []string{"scoped-commits"}; !slices.Equal(got.Skills(DefaultName), want) {
		t.Errorf("default = %v, want %v", got.Skills(DefaultName), want)
	}
	if want := []string{"dataviz", "tanstack-router"}; !slices.Equal(got.Skills("web"), want) {
		t.Errorf("web = %v, want %v", got.Skills("web"), want)
	}
	if want := []string{"web"}; !slices.Equal(got.Names(), want) {
		t.Errorf("Names() = %v, want %v", got.Names(), want)
	}
	// The manifest is dot-prefixed so library scans and harnesses ignore it.
	if base := filepath.Base(File(lib)); !strings.HasPrefix(base, ".") {
		t.Errorf("manifest %q must be hidden from library scans", base)
	}
}

// Sets are stored sorted and deduplicated so the manifest diffs cleanly in git.
func TestSetSkillsNormalizes(t *testing.T) {
	var s Store
	s.SetSkills("web", []string{"beta", "alpha", "beta"})
	if want := []string{"alpha", "beta"}; !slices.Equal(s.Skills("web"), want) {
		t.Errorf("got %v, want %v", s.Skills("web"), want)
	}
}

// Skills hands out copies: mutating one must not reach into the store.
func TestSkillsReturnsCopy(t *testing.T) {
	var s Store
	s.SetSkills("web", []string{"alpha"})
	got := s.Skills("web")
	got[0] = "clobbered"
	if s.Skills("web")[0] != "alpha" {
		t.Error("Skills must not alias the stored slice")
	}
}

func TestDefaultNameAndEmptyAreTheSameSet(t *testing.T) {
	var s Store
	s.SetSkills("", []string{"alpha"})
	if !slices.Equal(s.Skills(DefaultName), []string{"alpha"}) {
		t.Error(`"" must address the default set`)
	}
	if !s.Has("") || !s.Has(DefaultName) {
		t.Error("the default set always exists")
	}
	if s.Has("web") {
		t.Error("Has must be false for an unknown project")
	}
}

func TestDelete(t *testing.T) {
	var s Store
	s.SetSkills("web", []string{"alpha"})
	if !s.Delete("web") {
		t.Fatal("Delete must report the project was there")
	}
	if s.Delete("web") {
		t.Fatal("deleting twice must report false")
	}
	if s.Has("web") {
		t.Fatal("project still present after Delete")
	}
}

func TestValidName(t *testing.T) {
	for _, ok := range []string{"web", "side-project", "p2"} {
		if err := ValidName(ok); err != nil {
			t.Errorf("ValidName(%q) = %v, want nil", ok, err)
		}
	}
	for _, bad := range []string{DefaultName, "", "Web", "with space", "-lead", "trail-"} {
		if err := ValidName(bad); err == nil {
			t.Errorf("ValidName(%q) must fail", bad)
		}
	}
}

func TestActiveRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	active, err := Active()
	if err != nil {
		t.Fatalf("missing state file must not error: %v", err)
	}
	if active != "" {
		t.Errorf("Active() = %q, want empty", active)
	}

	if err := SetActive("web"); err != nil {
		t.Fatal(err)
	}
	if active, err = Active(); err != nil || active != "web" {
		t.Fatalf("Active() = %q, %v, want web", active, err)
	}

	// The default set is the absence of an active project, not a project.
	if err := SetActive(DefaultName); err != nil {
		t.Fatal(err)
	}
	if active, err = Active(); err != nil || active != "" {
		t.Fatalf("Active() = %q, %v, want empty", active, err)
	}
}

func TestStatePathIsMachineLocal(t *testing.T) {
	t.Setenv("HOME", "/custom")
	path, err := StatePath()
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join("/custom", ".memex", "state.toml") {
		t.Errorf("StatePath() = %q", path)
	}
}

// A manifest written by a newer memex must still load: it travels with the
// library between machines.
func TestLoadIgnoresUnknownKeys(t *testing.T) {
	lib := t.TempDir()
	manifest := "someday = true\n\n[default]\nskills = [\"alpha\"]\n\n[projects.web]\nskills = [\"beta\"]\nnotes = \"hi\"\n"
	if err := os.WriteFile(File(lib), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Load(lib)
	if err != nil {
		t.Fatalf("unknown keys must not error: %v", err)
	}
	if !slices.Equal(s.Skills("web"), []string{"beta"}) {
		t.Errorf("web = %v", s.Skills("web"))
	}
}
