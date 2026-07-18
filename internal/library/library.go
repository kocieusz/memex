// Package library scans and manages the skill library: the source-of-truth
// directory (~/.memex/skills by default) whose immediate subdirectories
// containing a SKILL.md are linkable skills.
package library

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// Skill is one linkable skill in the library.
type Skill struct {
	Name string
	Path string // absolute path to the skill directory
}

// DefaultDir returns the default library location, ~/.memex/skills.
func DefaultDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memex", "skills"), nil
}

// Scan returns the skills in dir: immediate subdirectories that contain a
// SKILL.md. Dot-prefixed entries are ignored.
func Scan(dir string) ([]Skill, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("reading library %s: %w", abs, err)
	}
	var skills []Skill
	for _, e := range entries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		path := filepath.Join(abs, e.Name())
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
			continue
		}
		skills = append(skills, Skill{Name: e.Name(), Path: path})
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

var nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// Scaffold creates dir/name/SKILL.md with minimal spec frontmatter and returns
// the path of the new skill directory.
func Scaffold(dir, name string) (string, error) {
	if !nameRe.MatchString(name) {
		return "", fmt.Errorf("invalid skill name %q: use lowercase letters, digits, and hyphens", name)
	}
	path := filepath.Join(dir, name)
	if _, err := os.Lstat(path); err == nil {
		return "", fmt.Errorf("%s already exists", path)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return "", err
	}
	content := fmt.Sprintf(`---
name: %s
description: TODO — when should an agent use this skill?
---

# %s

TODO
`, name, name)
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
