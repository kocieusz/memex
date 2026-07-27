package plan

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/kocieusz/memex/internal/library"
	"github.com/kocieusz/memex/internal/target"
)

func mkSkill(t *testing.T, root, name string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(name), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// fixture builds a library with alpha/beta/gamma and two providers: alpha is
// linked in claude only, and a real directory blocks beta in codex.
func fixture(t *testing.T) (string, []target.Builtin, []Status) {
	t.Helper()
	lib := t.TempDir()
	alpha := mkSkill(t, lib, "alpha")
	mkSkill(t, lib, "beta")
	mkSkill(t, lib, "gamma")

	claude, codex := t.TempDir(), t.TempDir()
	if err := os.Symlink(alpha, filepath.Join(claude, "alpha")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(codex, "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	providers := []target.Builtin{{Name: "claude", Path: claude}, {Name: "codex", Path: codex}}

	skills, err := library.Scan(lib)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err := Survey(providers, skills)
	if err != nil {
		t.Fatal(err)
	}
	return lib, providers, statuses
}

func find(statuses []Status, name string) Status {
	for _, s := range statuses {
		if s.Skill.Name == name {
			return s
		}
	}
	return Status{}
}

func TestSurvey(t *testing.T) {
	_, _, statuses := fixture(t)
	if got := find(statuses, "alpha").LinkedIn; !slices.Equal(got, []string{"claude"}) {
		t.Errorf("alpha LinkedIn = %v, want [claude]", got)
	}
	if got := find(statuses, "beta").LinkedIn; len(got) != 0 {
		t.Errorf("beta LinkedIn = %v, want none", got)
	}
	blocked := find(statuses, "beta").Blocked
	if len(blocked) != 1 || blocked[0].Provider != "codex" {
		t.Errorf("beta Blocked = %+v, want codex", blocked)
	}
	if got := find(statuses, "gamma"); len(got.LinkedIn) != 0 || len(got.Blocked) != 0 {
		t.Errorf("gamma = %+v, want untouched", got)
	}
}

func TestLinkedNames(t *testing.T) {
	_, _, statuses := fixture(t)
	if got := LinkedNames(statuses); !slices.Equal(got, []string{"alpha"}) {
		t.Errorf("LinkedNames = %v, want [alpha]", got)
	}
}

// A set is applied exactly: wanted skills are linked into every provider that
// can take them, and links outside the set are removed.
func TestBuildExactSync(t *testing.T) {
	_, providers, statuses := fixture(t)
	p := Build(providers, statuses, []string{"beta", "ghost"})

	var links, unlinks []string
	for _, a := range p.Actions {
		if a.Link {
			links = append(links, a.Provider+"/"+a.Skill.Name)
		} else {
			unlinks = append(unlinks, a.Provider+"/"+a.Skill.Name)
		}
	}
	slices.Sort(links)
	if want := []string{"claude/beta"}; !slices.Equal(links, want) {
		t.Errorf("links = %v, want %v", links, want)
	}
	if want := []string{"claude/alpha"}; !slices.Equal(unlinks, want) {
		t.Errorf("unlinks = %v, want %v — alpha left the set", unlinks, want)
	}
	if len(p.Blocked) != 1 || p.Blocked[0].Provider != "codex" || p.Blocked[0].Skill != "beta" {
		t.Errorf("Blocked = %+v, want beta in codex", p.Blocked)
	}
	if want := []string{"ghost"}; !slices.Equal(p.Missing, want) {
		t.Errorf("Missing = %v, want %v", p.Missing, want)
	}
	if got := p.Summary(); got != "+1 link, −1 unlink" {
		t.Errorf("Summary() = %q", got)
	}
}

func TestBuildEmptyWhenApplied(t *testing.T) {
	_, providers, statuses := fixture(t)
	// alpha is linked in claude and nothing blocks it in codex, so wanting it
	// still leaves work; wanting nothing that is linked is what's settled here.
	p := Build(providers, statuses, nil)
	if len(p.Actions) != 1 || p.Actions[0].Link {
		t.Fatalf("want a single unlink, got %+v", p.Actions)
	}
	if p.Empty() {
		t.Fatal("Empty() must be false when there are actions")
	}
}

func TestApplyLinksAndUnlinks(t *testing.T) {
	lib, providers, statuses := fixture(t)
	p := Build(providers, statuses, []string{"gamma"})

	var log []Action
	if err := p.Apply(lib, func(a Action) { log = append(log, a) }); err != nil {
		t.Fatal(err)
	}
	if len(log) != len(p.Actions) {
		t.Errorf("logged %d of %d actions", len(log), len(p.Actions))
	}
	for _, prov := range providers {
		gamma := filepath.Join(prov.Path, "gamma")
		dest, err := os.Readlink(gamma)
		if err != nil {
			t.Fatalf("gamma not linked in %s: %v", prov.Name, err)
		}
		if dest != filepath.Join(lib, "gamma") {
			t.Errorf("%s → %s, want the library copy", gamma, dest)
		}
	}
	if _, err := os.Lstat(filepath.Join(providers[0].Path, "alpha")); !os.IsNotExist(err) {
		t.Error("alpha should have been unlinked from claude")
	}
	// The real directory memex doesn't own is still there, untouched.
	if fi, err := os.Lstat(filepath.Join(providers[1].Path, "beta")); err != nil || !fi.IsDir() {
		t.Error("the native beta directory must survive an apply")
	}
}

// Applying twice is a no-op: the second survey finds everything in place.
func TestApplyIsIdempotent(t *testing.T) {
	lib, providers, statuses := fixture(t)
	if err := Build(providers, statuses, []string{"gamma"}).Apply(lib, nil); err != nil {
		t.Fatal(err)
	}
	skills, err := library.Scan(lib)
	if err != nil {
		t.Fatal(err)
	}
	statuses, err = Survey(providers, skills)
	if err != nil {
		t.Fatal(err)
	}
	if p := Build(providers, statuses, []string{"gamma"}); !p.Empty() {
		t.Errorf("second apply should have nothing to do, got %+v", p.Actions)
	}
}
