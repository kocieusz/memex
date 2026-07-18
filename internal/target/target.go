// Package target scans harness skills directories, classifies their entries,
// and performs the only mutations memex is allowed: creating symlinks into the
// library and removing symlinks that point into the library.
package target

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kocieusz/memex/internal/library"
)

// State classifies one entry of a target directory.
type State int

const (
	// Linked: a symlink resolving into the library. Toggleable.
	Linked State = iota
	// Available: a library skill not present in the target. Toggleable.
	Available
	// Native: a real directory owned by the target. Never touched.
	Native
	// Foreign: a symlink pointing outside the library. Never touched.
	Foreign
	// Broken: a symlink whose destination no longer exists.
	Broken
)

func (s State) String() string {
	switch s {
	case Linked:
		return "linked"
	case Available:
		return "available"
	case Native:
		return "native"
	case Foreign:
		return "foreign"
	case Broken:
		return "broken"
	}
	return "unknown"
}

// Entry is one row of a target scan.
type Entry struct {
	Name  string `json:"name"`
	State State  `json:"-"`
	// Dest is the symlink destination for linked/foreign/broken entries.
	Dest string `json:"dest,omitempty"`
	// Shadows is true for a native entry whose name collides with a library
	// skill; the library skill cannot be linked here.
	Shadows bool `json:"shadows,omitempty"`
}

// Toggleable reports whether memex may act on this entry.
func (e Entry) Toggleable() bool { return e.State == Linked || e.State == Available }

// Scan classifies every visible entry of targetDir and appends the library
// skills not present there as Available. A missing targetDir is an empty one.
func Scan(targetDir string, skills []library.Skill) ([]Entry, error) {
	targetDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}
	byName := make(map[string]library.Skill, len(skills))
	for _, s := range skills {
		byName[s.Name] = s
	}

	dirEntries, err := os.ReadDir(targetDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading target %s: %w", targetDir, err)
	}

	var entries []Entry
	seen := map[string]bool{}
	for _, de := range dirEntries {
		name := de.Name()
		if name[0] == '.' {
			continue
		}
		path := filepath.Join(targetDir, name)
		fi, err := os.Lstat(path)
		if err != nil {
			return nil, err
		}
		switch {
		case fi.Mode()&os.ModeSymlink != 0:
			e := classifySymlink(targetDir, path, name, skills)
			entries = append(entries, e)
			seen[name] = true
		case fi.IsDir():
			_, shadows := byName[name]
			entries = append(entries, Entry{Name: name, State: Native, Shadows: shadows})
			seen[name] = true
		}
		// plain files are invisible to memex
	}
	for _, s := range skills {
		if !seen[s.Name] {
			entries = append(entries, Entry{Name: s.Name, State: Available})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func classifySymlink(targetDir, path, name string, skills []library.Skill) Entry {
	dest, err := os.Readlink(path)
	if err != nil {
		return Entry{Name: name, State: Broken}
	}
	resolved := dest
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(targetDir, resolved)
	}
	if _, err := os.Stat(path); err != nil {
		return Entry{Name: name, State: Broken, Dest: dest}
	}
	for _, s := range skills {
		if SameDir(resolved, s.Path) {
			return Entry{Name: name, State: Linked, Dest: s.Path}
		}
	}
	return Entry{Name: name, State: Foreign, Dest: dest}
}

// SameDir compares two directory paths, tolerating symlinks in their parents
// (e.g. /tmp vs /private/tmp on macOS).
func SameDir(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ea, errA := filepath.EvalSymlinks(a)
	eb, errB := filepath.EvalSymlinks(b)
	return errA == nil && errB == nil && ea == eb
}

// Link creates targetDir/skill.Name -> skill.Path. Creating targetDir if
// needed. Linking an already-linked skill is a no-op; any other existing entry
// is refused.
func Link(targetDir string, skill library.Skill) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(targetDir, skill.Name)
	if fi, err := os.Lstat(path); err == nil {
		if fi.Mode()&os.ModeSymlink != 0 {
			if dest, err := os.Readlink(path); err == nil && SameDir(resolveDest(targetDir, dest), skill.Path) {
				return nil // already linked
			}
		}
		return fmt.Errorf("%s already exists and is not a link to the library — refusing to touch it", path)
	}
	return os.Symlink(skill.Path, path)
}

// Unlink removes targetDir/name only if it is a symlink resolving into
// libraryDir. This is the safety invariant: memex never deletes anything else.
func Unlink(targetDir, libraryDir, name string) error {
	path := filepath.Join(targetDir, name)
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s is not a symlink — refusing to remove", path)
	}
	dest, err := os.Readlink(path)
	if err != nil {
		return err
	}
	resolved := resolveDest(targetDir, dest)
	if !insideDir(resolved, libraryDir) {
		return fmt.Errorf("%s points outside the library (%s) — refusing to remove", path, dest)
	}
	return os.Remove(path)
}

func resolveDest(targetDir, dest string) string {
	if filepath.IsAbs(dest) {
		return dest
	}
	return filepath.Join(targetDir, dest)
}

func insideDir(path, dir string) bool {
	if SameDir(filepath.Dir(path), dir) {
		return true
	}
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
