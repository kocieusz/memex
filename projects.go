package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/kocieusz/memex/internal/project"
	"github.com/kocieusz/memex/internal/tui"
)

// projectCmd groups the project subcommands. Bare `memex project` opens a picker
// to switch between them.
type projectCmd struct {
	Switch projectSwitchCmd `cmd:"" default:"1" hidden:""`
	Ls     projectLsCmd     `cmd:"" help:"List the projects and the default set."`
	New    projectNewCmd    `cmd:"" help:"Create a project and pick its skills."`
	Edit   projectEditCmd   `cmd:"" help:"Change which skills a project holds."`
	Show   projectShowCmd   `cmd:"" help:"Print the skills in a project."`
	Use    projectUseCmd    `cmd:"" help:"Switch to a project and apply it everywhere."`
	Reset  projectResetCmd  `cmd:"" help:"Leave the active project and apply the default set."`
	Rm     projectRmCmd     `cmd:"" help:"Delete a project."`
}

type projectSwitchCmd struct{}

// Run offers the default set and every project, then applies the chosen one.
func (p *projectSwitchCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	names := append([]string{project.DefaultName}, s.store.Names()...)
	options := make([]tui.Option, len(names))
	for i, name := range names {
		detail := plural(len(s.store.Skills(name)), "skill")
		if name == s.setName() {
			detail += " · active"
		}
		options[i] = tui.Option{Label: name, Detail: detail}
	}
	opt, ok, err := tui.Pick("Apply which project?", options)
	if err != nil || !ok {
		return err
	}
	return s.activate(opt.Label)
}

// activate makes name the set in force and applies it to every provider.
func (s *session) activate(name string) error {
	if !s.store.Has(name) {
		return unknownProject(s, name)
	}
	if err := project.SetActive(name); err != nil {
		return err
	}
	if name == project.DefaultName {
		s.active = ""
		fmt.Println("using the default set")
	} else {
		s.active = name
		fmt.Printf("switched to project %s\n", name)
	}
	return s.sync()
}

func unknownProject(s *session, name string) error {
	known := strings.Join(append([]string{project.DefaultName}, s.store.Names()...), ", ")
	return fmt.Errorf("no project %q — known sets: %s (create one with `memex project new %s`)", name, known, name)
}

type projectLsCmd struct{}

func (p *projectLsCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	names := append([]string{project.DefaultName}, s.store.Names()...)
	nameW := 0
	for _, name := range names {
		nameW = max(nameW, len(name))
	}
	for _, name := range names {
		marker, tone := " ", tui.Muted
		if name == s.setName() {
			marker, tone = "*", tui.Good
		}
		detail := plural(len(s.store.Skills(name)), "skill")
		fmt.Printf("%s %s %s\n", marker, tui.Badge(tone, fmt.Sprintf("%-*s", nameW, name)), tui.Dim(detail))
	}
	return nil
}

type projectNewCmd struct {
	Name string `arg:"" help:"Name of the new project (lowercase, hyphens)."`
	Use  bool   `help:"Apply the project as soon as it is created."`
}

func (p *projectNewCmd) Run(c *cli) error {
	if err := project.ValidName(p.Name); err != nil {
		return err
	}
	s, err := c.open()
	if err != nil {
		return err
	}
	if s.store.Has(p.Name) {
		return fmt.Errorf("project %q already exists — edit it with `memex project edit %s`", p.Name, p.Name)
	}
	// An empty set exists from here on, so quitting the checklist still leaves
	// the project created — just without skills.
	s.store.SetSkills(p.Name, nil)
	if err := project.Save(s.source, s.store); err != nil {
		return err
	}
	fmt.Printf("created project %s\n", p.Name)
	if err := s.edit(p.Name); err != nil {
		return err
	}
	if p.Use {
		return s.activate(p.Name)
	}
	fmt.Printf("run `memex use %s` to apply it\n", p.Name)
	return nil
}

type projectEditCmd struct {
	Name string `arg:"" optional:"" help:"Project to edit; defaults to the set in force."`
}

func (p *projectEditCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	name := p.Name
	if name == "" {
		name = s.setName()
	}
	if !s.store.Has(name) {
		return unknownProject(s, name)
	}
	return s.edit(name)
}

type projectShowCmd struct {
	Name string `arg:"" optional:"" help:"Project to print; defaults to the set in force."`
}

func (p *projectShowCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	name := p.Name
	if name == "" {
		name = s.setName()
	}
	if !s.store.Has(name) {
		return unknownProject(s, name)
	}
	inLibrary := map[string]bool{}
	for _, sk := range s.skills {
		inLibrary[sk.Name] = true
	}
	for _, skill := range s.store.Skills(name) {
		if !inLibrary[skill] {
			fmt.Printf("%s %s\n", tui.Badge(tui.Bad, "missing"), skill)
			continue
		}
		fmt.Printf("%s %s\n", tui.Badge(tui.Good, "skill"), skill)
	}
	return nil
}

type projectUseCmd struct {
	Name string `arg:"" help:"Project to apply, or \"default\"."`
}

func (p *projectUseCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	return s.activate(p.Name)
}

type projectResetCmd struct{}

func (p *projectResetCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	return s.activate(project.DefaultName)
}

type projectRmCmd struct {
	Name string `arg:"" help:"Project to delete."`
}

func (p *projectRmCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	if p.Name == project.DefaultName {
		return fmt.Errorf("the default set cannot be deleted — empty it with `memex` instead")
	}
	if !s.store.Has(p.Name) {
		return unknownProject(s, p.Name)
	}
	if p.Name == s.active {
		return fmt.Errorf("project %q is active — run `memex reset` first, so its skills get unlinked", p.Name)
	}
	s.store.Delete(p.Name)
	if err := project.Save(s.source, s.store); err != nil {
		return err
	}
	fmt.Printf("deleted project %s\n", p.Name)
	return nil
}

// useCmd and resetCmd are top-level shortcuts for the two project commands
// reached most often.
type useCmd struct {
	Name string `arg:"" help:"Project to apply, or \"default\"."`
}

func (u *useCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	return s.activate(u.Name)
}

type resetCmd struct{}

func (r *resetCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	if s.active == "" {
		fmt.Fprintln(os.Stderr, "no project is active; re-applying the default set")
	}
	return s.activate(project.DefaultName)
}
