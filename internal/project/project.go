// Package project stores named skill sets — projects — and tracks which one is
// active. Definitions live in the library (.projects.toml) so they are
// versioned alongside the skills they name; the active project is machine-local
// state in ~/.memex/state.toml, since it says what this machine is working on.
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/BurntSushi/toml"
)

// DefaultName is the reserved name of the fallback set, applied whenever no
// project is active.
const DefaultName = "default"

// fileName is the manifest kept in the library root; the dot prefix keeps it
// invisible to library scans and harnesses.
const fileName = ".projects.toml"

// Set is one named collection of library skill names.
type Set struct {
	Skills []string `toml:"skills"`
}

// Store is the whole manifest: the default set plus the named projects.
type Store struct {
	Default  Set            `toml:"default"`
	Projects map[string]Set `toml:"projects,omitempty"`

	// Found reports whether the library actually had a manifest. It is false on
	// a first run, which lets callers seed the default set from the links that
	// already exist instead of treating them as strays to remove.
	Found bool `toml:"-"`
}

// File returns the manifest path for libraryDir.
func File(libraryDir string) string {
	return filepath.Join(libraryDir, fileName)
}

// Load reads the manifest. A missing file yields an empty store with Found
// false. Unknown keys are ignored: the manifest travels between machines that
// may run different memex versions.
func Load(libraryDir string) (Store, error) {
	s := Store{Projects: map[string]Set{}}
	_, err := toml.DecodeFile(File(libraryDir), &s)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return Store{}, fmt.Errorf("reading %s: %w", File(libraryDir), err)
	}
	if s.Projects == nil {
		s.Projects = map[string]Set{}
	}
	s.Found = true
	return s, nil
}

// Save writes the manifest.
func Save(libraryDir string, s Store) error {
	f, err := os.Create(File(libraryDir))
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(s); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

// Names returns the project names, sorted. The default set is not one of them.
func (s Store) Names() []string {
	names := make([]string, 0, len(s.Projects))
	for name := range s.Projects {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Has reports whether name identifies a set. The default set always exists.
func (s Store) Has(name string) bool {
	if isDefault(name) {
		return true
	}
	_, ok := s.Projects[name]
	return ok
}

// Skills returns a copy of the skill names in a set; an empty name or
// DefaultName means the default set. An unknown project has no skills.
func (s Store) Skills(name string) []string {
	if isDefault(name) {
		return slices.Clone(s.Default.Skills)
	}
	return slices.Clone(s.Projects[name].Skills)
}

// SetSkills replaces a set's skills, stored sorted and deduplicated so the
// manifest diffs cleanly in git.
func (s *Store) SetSkills(name string, skills []string) {
	set := Set{Skills: normalize(skills)}
	if isDefault(name) {
		s.Default = set
		return
	}
	if s.Projects == nil {
		s.Projects = map[string]Set{}
	}
	s.Projects[name] = set
}

// Delete removes a project, reporting whether it was there.
func (s *Store) Delete(name string) bool {
	if _, ok := s.Projects[name]; !ok {
		return false
	}
	delete(s.Projects, name)
	return true
}

func isDefault(name string) bool { return name == "" || name == DefaultName }

func normalize(skills []string) []string {
	out := slices.Clone(skills)
	slices.Sort(out)
	return slices.Compact(out)
}

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidName rejects names a project can't have: the reserved default name, and
// anything that isn't lowercase words joined by hyphens.
func ValidName(name string) error {
	if name == DefaultName {
		return fmt.Errorf("%q is reserved for the fallback set", DefaultName)
	}
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid project name %q: use lowercase letters, digits, and hyphens", name)
	}
	return nil
}

// state is the machine-local file recording which project is active.
type state struct {
	Active string `toml:"active"`
}

// StatePath returns ~/.memex/state.toml.
func StatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memex", "state.toml"), nil
}

// Active returns the active project name, or "" when the default set is in use.
func Active() (string, error) {
	path, err := StatePath()
	if err != nil {
		return "", err
	}
	var st state
	_, err = toml.DecodeFile(path, &st)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading %s: %w", path, err)
	}
	if st.Active == DefaultName {
		return "", nil
	}
	return st.Active, nil
}

// SetActive records the active project. An empty name or DefaultName clears it,
// putting the default set back in charge.
func SetActive(name string) error {
	path, err := StatePath()
	if err != nil {
		return err
	}
	if name == DefaultName {
		name = ""
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if err := toml.NewEncoder(f).Encode(state{Active: name}); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
