// Package plan works out what has to change for one skill set to be linked into
// every provider memex manages, and applies it. A set is applied exactly: its
// skills are linked everywhere, and library links that are not in it are
// removed. Anything memex doesn't own — native directories, foreign links — is
// reported as blocking, never touched.
package plan

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/kocieusz/memex/internal/library"
	"github.com/kocieusz/memex/internal/target"
)

// Status is one library skill's situation across the providers.
type Status struct {
	Skill library.Skill
	// LinkedIn names the providers holding a link to this skill.
	LinkedIn []string
	// Blocked names the providers where the skill's name is taken by something
	// memex must not touch.
	Blocked []Block
}

// Block is a provider where a skill cannot be linked, and why.
type Block struct {
	Provider string
	Skill    string
	Reason   string
}

// Survey scans every provider and reports where each library skill stands.
func Survey(providers []target.Builtin, skills []library.Skill) ([]Status, error) {
	statuses := make([]Status, len(skills))
	index := make(map[string]int, len(skills))
	for i, s := range skills {
		statuses[i] = Status{Skill: s}
		index[s.Name] = i
	}
	for _, p := range providers {
		entries, err := target.Scan(p.Path, skills)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			i, ok := index[e.Name]
			if !ok {
				continue // an entry of the provider that no library skill claims
			}
			st := &statuses[i]
			switch e.State {
			case target.Linked:
				st.LinkedIn = append(st.LinkedIn, p.Name)
			case target.Native:
				st.Blocked = append(st.Blocked, Block{p.Name, e.Name, "a real directory is in the way"})
			case target.Foreign:
				st.Blocked = append(st.Blocked, Block{p.Name, e.Name, "a link to " + e.Dest + " is in the way"})
			case target.Broken:
				st.Blocked = append(st.Blocked, Block{p.Name, e.Name, "a broken link is in the way — run `memex doctor --fix`"})
			case target.Available:
			}
		}
	}
	return statuses, nil
}

// LinkedNames returns every skill that is linked in at least one provider,
// sorted. It seeds the default set on a first run, so adopting projects doesn't
// unlink what you already had.
func LinkedNames(statuses []Status) []string {
	var names []string
	for _, st := range statuses {
		if len(st.LinkedIn) > 0 {
			names = append(names, st.Skill.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Action is one symlink to create or remove.
type Action struct {
	Provider string
	Dir      string
	Skill    library.Skill
	// Link is true to create the link, false to remove it.
	Link bool
}

// Plan is everything an apply will do, plus what it can't.
type Plan struct {
	Actions []Action
	// Blocked lists wanted skills that some provider won't accept.
	Blocked []Block
	// Missing lists set members the library doesn't have.
	Missing []string
}

// Build compares the wanted skill set against a survey and returns the actions
// that bring every provider in line with it.
func Build(providers []target.Builtin, statuses []Status, want []string) Plan {
	wanted := make(map[string]bool, len(want))
	for _, n := range want {
		wanted[n] = true
	}
	var p Plan
	known := make(map[string]bool, len(statuses))
	for _, st := range statuses {
		known[st.Skill.Name] = true
	}
	for n := range wanted {
		if !known[n] {
			p.Missing = append(p.Missing, n)
		}
	}
	sort.Strings(p.Missing)

	for _, prov := range providers {
		for _, st := range statuses {
			linked := slices.Contains(st.LinkedIn, prov.Name)
			switch {
			case wanted[st.Skill.Name] && !linked:
				if b, ok := blockFor(st, prov.Name); ok {
					p.Blocked = append(p.Blocked, b)
					continue
				}
				p.Actions = append(p.Actions, Action{Provider: prov.Name, Dir: prov.Path, Skill: st.Skill, Link: true})
			case !wanted[st.Skill.Name] && linked:
				p.Actions = append(p.Actions, Action{Provider: prov.Name, Dir: prov.Path, Skill: st.Skill})
			}
		}
	}
	return p
}

func blockFor(st Status, provider string) (Block, bool) {
	for _, b := range st.Blocked {
		if b.Provider == provider {
			return b, true
		}
	}
	return Block{}, false
}

func (p Plan) Empty() bool { return len(p.Actions) == 0 }

// Summary describes the plan in one line, e.g. "+4 link, −1 unlink".
func (p Plan) Summary() string {
	links, unlinks := 0, 0
	for _, a := range p.Actions {
		if a.Link {
			links++
		} else {
			unlinks++
		}
	}
	var parts []string
	if links > 0 {
		parts = append(parts, fmt.Sprintf("+%d link", links))
	}
	if unlinks > 0 {
		parts = append(parts, fmt.Sprintf("−%d unlink", unlinks))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

// Apply performs the actions, calling log after each one. libraryDir is the
// safety boundary: only links pointing inside it are ever removed.
func (p Plan) Apply(libraryDir string, log func(Action)) error {
	for _, a := range p.Actions {
		var err error
		if a.Link {
			err = target.Link(a.Dir, a.Skill)
		} else {
			err = target.Unlink(a.Dir, libraryDir, a.Skill.Name)
		}
		if err != nil {
			return err
		}
		if log != nil {
			log(a)
		}
	}
	return nil
}
