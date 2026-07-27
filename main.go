// memex links Agent Skills from a git-versioned library (~/.memex/skills by
// default) into harness skills directories via symlinks.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"

	"github.com/kocieusz/memex/internal/config"
	"github.com/kocieusz/memex/internal/library"
	"github.com/kocieusz/memex/internal/origin"
	"github.com/kocieusz/memex/internal/plan"
	"github.com/kocieusz/memex/internal/project"
	"github.com/kocieusz/memex/internal/release"
	"github.com/kocieusz/memex/internal/remote"
	"github.com/kocieusz/memex/internal/target"
	"github.com/kocieusz/memex/internal/tui"
)

type cli struct {
	Source  string           `help:"Skill library directory (overrides the config file; default ~/.memex/skills)." env:"MEMEX_SOURCE" placeholder:"DIR"`
	Version kong.VersionFlag `help:"Print version and exit."`

	Manage    manageCmd    `cmd:"" default:"1" hidden:""`
	Sync      syncCmd      `cmd:"" help:"Re-link the active set into every provider."`
	Project   projectCmd   `cmd:"" help:"Create, switch, and edit projects — named sets of skills."`
	Use       useCmd       `cmd:"" help:"Switch to a project and apply it everywhere."`
	Reset     resetCmd     `cmd:"" help:"Leave the active project and apply the default set."`
	Providers providersCmd `cmd:"" help:"List providers, or enable and disable them."`
	Ls        listCmd      `cmd:"" aliases:"list" help:"List library skills, or a target's skill states."`
	Adopt     adoptCmd     `cmd:"" help:"Move a real skill directory into the library and leave a symlink."`
	Clone     cloneCmd     `cmd:"" help:"Pick skills from a git repo and copy them into the library."`
	Touch     newCmd       `cmd:"" aliases:"new" help:"Scaffold a new skill in the library."`
	Doctor    doctorCmd    `cmd:"" help:"Check providers, projects, and library for problems."`
	Upgrade   upgradeCmd   `cmd:"" aliases:"update" help:"Update memex to the latest release."`
}

func main() {
	var c cli
	ctx := kong.Parse(&c,
		kong.Name("memex"),
		kong.Description("Manage Agent Skills across harnesses with symlinks from one library."),
		kong.UsageOnError(),
		// One line per command, and only the next level down: `memex --help`
		// lists `project`, `memex project --help` lists what it holds.
		kong.ConfigureHelp(kong.HelpOptions{Compact: true, NoExpandSubcommands: true}),
		kong.Help(colorHelp),
		kong.Vars{"version": version()},
	)
	ctx.FatalIfErrorf(ctx.Run(&c))
}

// buildVersion is stamped in by goreleaser (-X main.buildVersion=…). It's empty
// in `go install` and local builds, which fall back to the module version.
var buildVersion string

// version reports the release this binary came from: the goreleaser tag for
// release builds, the module version for `go install` builds, else (devel).
func version() string {
	if buildVersion != "" {
		return buildVersion
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" {
		return bi.Main.Version
	}
	return "unknown"
}

// colorHelp renders kong's default help, then tints section headings and
// command names so the overview is easier to scan.
func colorHelp(options kong.HelpOptions, ctx *kong.Context) error {
	var buf bytes.Buffer
	out := ctx.Stdout
	ctx.Stdout = &buf
	err := kong.DefaultHelpPrinter(options, ctx)
	ctx.Stdout = out
	if err != nil {
		return err
	}
	headStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	inCommands := false
	for line := range strings.SplitSeq(strings.TrimRight(buf.String(), "\n"), "\n") {
		switch {
		case line != "" && !strings.HasPrefix(line, " ") && strings.HasSuffix(line, ":"):
			inCommands = line == "Commands:"
			fmt.Fprintln(out, headStyle.Render(line))
		case inCommands && strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   "):
			// Compact help is two columns split by run of spaces; tint the whole
			// command path, which for a subcommand is more than one word.
			name, rest, found := strings.Cut(line[2:], "  ")
			if !found {
				name, rest, _ = strings.Cut(line[2:], " ")
			}
			fmt.Fprintln(out, "  "+cmdStyle.Render(name)+"  "+rest)
		default:
			fmt.Fprintln(out, line)
		}
	}
	return nil
}

// resolveSource picks the library directory — --source/MEMEX_SOURCE, then the
// config file, then the default — and normalises it to an absolute path
// without requiring it to exist.
func (c *cli) resolveSource() (string, error) {
	dir := c.Source
	if dir == "" {
		cfg, err := config.Load()
		if err != nil {
			return "", err
		}
		dir = cfg.Library
	}
	if dir == "" {
		var err error
		if dir, err = library.DefaultDir(); err != nil {
			return "", err
		}
	}
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[2:])
	}
	return filepath.Abs(dir)
}

// sourceDir resolves the library directory and verifies it exists.
func (c *cli) sourceDir() (string, error) {
	abs, err := c.resolveSource()
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		cfgPath, _ := config.Path()
		return "", fmt.Errorf("skill library %s does not exist — start one with `memex touch <name>` or `memex clone <repo>`, or point `library` in %s at an existing directory", abs, cfgPath)
	}
	return abs, nil
}

// sourceDirCreate resolves the library directory, creating it if missing —
// for commands that add skills to the library.
func (c *cli) sourceDirCreate() (string, error) {
	abs, err := c.resolveSource()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	return abs, nil
}

func (c *cli) scan(targetDir string) (string, []library.Skill, []target.Entry, error) {
	source, err := c.sourceDir()
	if err != nil {
		return "", nil, nil, err
	}
	skills, err := library.Scan(source)
	if err != nil {
		return "", nil, nil, err
	}
	entries, err := target.Scan(targetDir, skills)
	return source, skills, entries, err
}

// session is everything the set-oriented commands work from: the library, the
// providers memex manages, the stored sets, and what is linked where.
type session struct {
	source    string
	skills    []library.Skill
	providers []target.Builtin
	store     project.Store
	// active is the project in force, or "" when the default set is.
	active   string
	statuses []plan.Status
}

// open gathers the session. On a machine that has never used projects it seeds
// the default set from whatever is already linked, so the first apply doesn't
// unlink the skills you had before projects existed.
func (c *cli) open() (*session, error) {
	source, err := c.sourceDir()
	if err != nil {
		return nil, err
	}
	skills, err := library.Scan(source)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	providers := target.Enabled(cfg.DisabledProviders)
	if len(providers) == 0 {
		return nil, noProvidersErr(cfg.DisabledProviders)
	}
	store, err := project.Load(source)
	if err != nil {
		return nil, err
	}
	active, err := project.Active()
	if err != nil {
		return nil, err
	}
	if active != "" && !store.Has(active) {
		fmt.Fprintf(os.Stderr, "warning: active project %q is not in %s — falling back to the default set\n", active, project.File(source))
		if err := project.SetActive(""); err != nil {
			return nil, err
		}
		active = ""
	}
	statuses, err := plan.Survey(providers, skills)
	if err != nil {
		return nil, err
	}
	s := &session{source: source, skills: skills, providers: providers, store: store, active: active, statuses: statuses}
	if !store.Found && active == "" {
		s.store.SetSkills(project.DefaultName, plan.LinkedNames(statuses))
	}
	return s, nil
}

// noProvidersErr explains an empty provider list — nothing detected, or
// everything detected disabled.
func noProvidersErr(disabled []string) error {
	if len(target.Builtins()) == 0 {
		return fmt.Errorf("no providers detected (looked for ~/.claude, ~/.codex, ~/.pi, ~/.agents)")
	}
	return fmt.Errorf("every detected provider is disabled (%s) — enable one with `memex providers enable <name>`", strings.Join(disabled, ", "))
}

// setName is the name of the set in force: the active project, or "default".
func (s *session) setName() string {
	if s.active == "" {
		return project.DefaultName
	}
	return s.active
}

func (s *session) providerNames() []string { return target.Names(s.providers) }

// rows builds the checklist for a set: every library skill, plus any name the
// set holds that the library no longer has.
func (s *session) rows(want []string) []tui.Row {
	inSet := make(map[string]bool, len(want))
	for _, n := range want {
		inSet[n] = true
	}
	rows := make([]tui.Row, 0, len(s.statuses))
	for _, st := range s.statuses {
		r := tui.Row{Name: st.Skill.Name, InSet: inSet[st.Skill.Name], LinkedIn: st.LinkedIn}
		for _, b := range st.Blocked {
			r.Blocked = append(r.Blocked, tui.Blocker{Provider: b.Provider, Reason: b.Reason})
		}
		rows = append(rows, r)
		delete(inSet, st.Skill.Name)
	}
	missing := make([]string, 0, len(inSet))
	for name := range inSet {
		missing = append(missing, name)
	}
	sort.Strings(missing)
	for _, name := range missing {
		rows = append(rows, tui.Row{Name: name, InSet: true, Missing: true})
	}
	return rows
}

// edit runs the checklist for the named set and saves what the user confirms.
// The set is only linked into the providers when it is the one in force.
func (s *session) edit(name string) error {
	live := name == s.setName()
	model := tui.NewModel(name, shortHome(s.source), s.providerNames(), s.rows(s.store.Skills(name)))
	model.Live = live
	final, err := tui.Run(model)
	if err != nil {
		return err
	}
	if !final.Accepted {
		fmt.Println("no changes")
		return nil
	}
	return s.save(name, final.Set())
}

// save stores a set and, when it is the one in force, brings every provider in
// line with it.
func (s *session) save(name string, want []string) error {
	s.store.SetSkills(name, want)
	if err := project.Save(s.source, s.store); err != nil {
		return err
	}
	s.store.Found = true
	if name != s.setName() {
		fmt.Printf("saved project %s (%s)\n", name, plural(len(want), "skill"))
		return nil
	}
	return s.sync()
}

// sync applies the set in force to every provider.
func (s *session) sync() error {
	want := s.store.Skills(s.setName())
	p := plan.Build(s.providers, s.statuses, want)
	for _, name := range p.Missing {
		fmt.Fprintf(os.Stderr, "warning: %s names %q, which is not in the library\n", s.setName(), name)
	}
	for _, b := range p.Blocked {
		fmt.Fprintf(os.Stderr, "warning: %s: cannot link %s — %s\n", b.Provider, b.Skill, b.Reason)
	}
	if p.Empty() {
		note := ""
		if len(p.Blocked) > 0 {
			note = fmt.Sprintf(", apart from %s in the way above", plural(len(p.Blocked), "name"))
		}
		fmt.Printf("%s is applied to %s%s\n", s.setName(), strings.Join(s.providerNames(), ", "), note)
		return s.recordSeeded(want)
	}
	err := p.Apply(s.source, func(a plan.Action) {
		verb := "linked"
		if !a.Link {
			verb = "unlinked"
		}
		fmt.Printf("%-8s %-8s %s\n", verb, a.Provider, a.Skill.Name)
	})
	if err != nil {
		return err
	}
	return s.recordSeeded(want)
}

// recordSeeded writes down a default set that was seeded from existing links, so
// from now on memex applies a set the library declares instead of re-deriving
// one. An empty seed is left alone: writing it would turn the next sync into an
// unlink-everything.
func (s *session) recordSeeded(want []string) error {
	if s.store.Found || len(want) == 0 {
		return nil
	}
	if err := project.Save(s.source, s.store); err != nil {
		return err
	}
	s.store.Found = true
	fmt.Printf("recorded the default set in %s (%s)\n", shortHome(project.File(s.source)), plural(len(want), "skill"))
	return nil
}

// plural renders a count with its noun: "1 skill", "2 skills".
func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// shortHome abbreviates the user's home directory to ~ for display.
func shortHome(path string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if rel, err := filepath.Rel(home, path); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.Join("~", rel)
	}
	return path
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

type manageCmd struct{}

// Run opens the checklist for the set in force — the default set, or the active
// project — and applies it to every provider at once.
func (m *manageCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	return s.edit(s.setName())
}

type syncCmd struct{}

// Run re-applies the set in force without opening the TUI, which is what you
// want after installing a new harness or pulling someone else's projects.
func (sc *syncCmd) Run(c *cli) error {
	s, err := c.open()
	if err != nil {
		return err
	}
	return s.sync()
}

type listCmd struct {
	Target string `help:"Target name (claude, codex, …) or path. Without it, the library is listed."`
	All    bool   `short:"a" help:"Also list entries not managed by memex (native dirs, foreign links)."`
	JSON   bool   `help:"Emit JSON."`
}

func (l *listCmd) Run(c *cli) error {
	if l.Target == "" {
		source, err := c.sourceDir()
		if err != nil {
			return err
		}
		return l.listLibrary(source)
	}
	dir, err := target.Resolve(l.Target)
	if err != nil {
		return err
	}
	if source, err := c.sourceDir(); err == nil && target.SameDir(dir, source) {
		return l.listLibrary(source)
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		fmt.Fprintf(os.Stderr, "warning: %s does not exist — listing library skills as available\n", dir)
	}
	_, _, all, err := c.scan(dir)
	if err != nil {
		return err
	}
	var entries []target.Entry
	for _, e := range all {
		if l.All || e.State == target.Linked || e.State == target.Available || e.State == target.Broken {
			entries = append(entries, e)
		}
	}
	if l.JSON {
		type jsonEntry struct {
			target.Entry
			State string `json:"state"`
		}
		out := make([]jsonEntry, len(entries))
		for i, e := range entries {
			out[i] = jsonEntry{e, e.State.String()}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, e := range entries {
		extra := ""
		if e.State == target.Foreign || e.State == target.Broken {
			extra = " → " + e.Dest
		}
		if e.Shadows {
			extra = " (shadows a library skill)"
		}
		fmt.Printf("%s %s%s\n", tui.StateBadge(e.State), e.Name, extra)
	}
	return nil
}

// listLibrary lists the library's own skills with their clone origins.
func (l *listCmd) listLibrary(source string) error {
	skills, err := library.Scan(source)
	if err != nil {
		return err
	}
	origins, err := origin.Load(source)
	if err != nil {
		return err
	}
	if l.JSON {
		type jsonSkill struct {
			Name  string `json:"name"`
			State string `json:"state"`
			Repo  string `json:"repo,omitempty"`
			Path  string `json:"path,omitempty"`
			Hash  string `json:"hash,omitempty"`
		}
		out := make([]jsonSkill, len(skills))
		for i, s := range skills {
			o := origins[s.Name]
			out[i] = jsonSkill{s.Name, "library", o.Repo, o.Path, o.Hash}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	nameW := 0
	for _, s := range skills {
		nameW = max(nameW, len(s.Name))
	}
	for _, s := range skills {
		extra := ""
		if o, ok := origins[s.Name]; ok {
			extra = "  " + tui.Dim("from "+shortRepo(o.Repo))
		}
		fmt.Printf("%s %-*s%s\n", tui.LibraryBadge(), nameW, s.Name, extra)
	}
	return nil
}

// shortRepo compresses a github.com clone URL to owner/repo for display.
func shortRepo(url string) string {
	for _, p := range []string{"https://github.com/", "http://github.com/", "git@github.com:"} {
		if after, ok := strings.CutPrefix(url, p); ok {
			return strings.TrimSuffix(after, ".git")
		}
	}
	return url
}

type adoptCmd struct {
	Path   string `arg:"" help:"Existing skill directory to move into the library."`
	DryRun bool   `help:"Show what would happen without doing it."`
}

func (a *adoptCmd) Run(c *cli) error {
	source, err := c.sourceDirCreate()
	if err != nil {
		return err
	}
	path, err := filepath.Abs(a.Path)
	if err != nil {
		return err
	}
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%s is already a symlink", path)
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err != nil {
		return fmt.Errorf("%s has no SKILL.md — not a skill", path)
	}
	name := filepath.Base(path)
	dest := filepath.Join(source, name)
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("library already has a skill named %q", name)
	}
	fmt.Printf("move %s → %s, then symlink back\n", path, dest)
	if a.DryRun {
		return nil
	}
	if !confirm("proceed?") {
		return nil
	}
	if err := os.Rename(path, dest); err != nil {
		return err
	}
	if err := os.Symlink(dest, path); err != nil {
		return fmt.Errorf("moved to library but linking back failed: %w", err)
	}
	fmt.Printf("adopted %s\n", name)
	return nil
}

type cloneCmd struct {
	Repo   string `arg:"" help:"Repo to fetch skills from: owner/repo, a clone URL, or a GitHub /tree/<branch>[/dir] link."`
	Branch string `short:"b" help:"Branch to clone (overrides one parsed from the URL)." placeholder:"NAME"`
}

// Run shallow-clones the repo, discovers the skills inside, and copies the
// ones picked in the TUI into the library.
func (cl *cloneCmd) Run(c *cli) error {
	source, err := c.sourceDirCreate()
	if err != nil {
		return err
	}
	ref, err := remote.ParseRef(cl.Repo)
	if err != nil {
		return err
	}
	if cl.Branch != "" {
		ref.Branch = cl.Branch
	}
	at := ""
	if ref.Branch != "" {
		at = " @ " + ref.Branch
	}
	fmt.Printf("cloning %s%s…\n", ref.URL, at)
	dir, cleanup, err := remote.Clone(ref)
	if err != nil {
		return err
	}
	defer cleanup()
	found, err := remote.Discover(dir, ref)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		return fmt.Errorf("no skills (directories with a SKILL.md) found in %s", cl.Repo)
	}

	skills, err := library.Scan(source)
	if err != nil {
		return err
	}
	origins, err := origin.Load(source)
	if err != nil {
		return err
	}
	items, updates, hashes, err := classifyClone(found, skills, source, origins, ref.URL)
	if err != nil {
		return err
	}

	sel, ok, err := tui.MultiSelect("Skills in "+cl.Repo, items)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Println("no changes")
		return nil
	}
	for _, i := range sel {
		f := found[i]
		verb := "copied"
		if updates[i] {
			verb = "updated"
			err = remote.Update(f, source)
		} else {
			err = remote.Copy(f, source)
		}
		if err != nil {
			return err
		}
		origins[f.Name] = origin.Origin{Repo: ref.URL, Path: f.Rel, Hash: hashes[i]}
		fmt.Printf("%-8s %s\n", verb, f.Name)
	}
	if err := origin.Save(source, origins); err != nil {
		return err
	}
	fmt.Println("run `memex` to link them into a harness")
	return nil
}

// classifyClone builds the picker rows for the skills found in a clone. Rows
// colliding with the library are unselectable — except when the library copy
// was cloned from this same repo and upstream changed, which becomes a
// selectable update. Returns the rows, an is-update flag per row, and the
// upstream content hash per row.
func classifyClone(found []remote.Skill, skills []library.Skill, source string, origins map[string]origin.Origin, repoURL string) ([]tui.MultiItem, []bool, []string, error) {
	inLib := make(map[string]bool, len(skills))
	for _, s := range skills {
		inLib[s.Name] = true
	}
	items := make([]tui.MultiItem, len(found))
	updates := make([]bool, len(found))
	hashes := make([]string, len(found))
	seen := map[string]string{} // name → rel of the first selectable occurrence
	for i, f := range found {
		items[i] = tui.MultiItem{Name: f.Name, Note: f.Rel, Desc: f.Description}
		hash, err := origin.HashDir(f.Path)
		if err != nil {
			return nil, nil, nil, err
		}
		hashes[i] = hash
		o, tracked := origins[f.Name]
		switch {
		case seen[f.Name] != "":
			items[i].Conflict = "duplicate name — see " + seen[f.Name]
		case inLib[f.Name] && tracked && o.Repo == repoURL:
			if hash == o.Hash {
				items[i].Conflict = "up to date"
				break
			}
			updates[i] = true
			note := "update available"
			if localHash, err := origin.HashDir(filepath.Join(source, f.Name)); err == nil && localHash != o.Hash {
				note = "update — overwrites local edits"
			}
			items[i].Note = f.Rel + "  · " + note
			seen[f.Name] = f.Rel
		case inLib[f.Name]:
			items[i].Conflict = "already in the library"
		default:
			seen[f.Name] = f.Rel
		}
	}
	return items, updates, hashes, nil
}

type newCmd struct {
	Name string `arg:"" help:"Name of the new skill (lowercase, hyphens)."`
}

func (n *newCmd) Run(c *cli) error {
	source, err := c.sourceDirCreate()
	if err != nil {
		return err
	}
	path, err := library.Scaffold(source, n.Name)
	if err != nil {
		return err
	}
	fmt.Printf("created %s\n", filepath.Join(path, "SKILL.md"))
	return nil
}

type doctorCmd struct {
	Fix bool `help:"Remove broken symlinks."`
}

func (d *doctorCmd) Run(c *cli) error {
	source, err := c.sourceDir()
	if err != nil {
		return err
	}
	skills, err := library.Scan(source)
	if err != nil {
		return err
	}
	// fixable counts the problems --fix can deal with, so the closing hint only
	// shows up when running it would help.
	problems, fixable := 0, 0

	libEntries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, e := range libEntries {
		if !e.IsDir() || e.Name()[0] == '.' {
			continue
		}
		if _, err := os.Stat(filepath.Join(source, e.Name(), "SKILL.md")); err != nil {
			fmt.Printf("library: %s has no SKILL.md\n", e.Name())
			problems++
		}
	}

	origins, err := origin.Load(source)
	if err != nil {
		return err
	}
	var stale []string
	for name := range origins {
		if _, err := os.Lstat(filepath.Join(source, name)); err != nil {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		problems++
		fixable++
		if d.Fix {
			delete(origins, name)
			fmt.Printf("library: pruned origin entry for missing skill %s\n", name)
		} else {
			fmt.Printf("library: origin entry for missing skill %s\n", name)
		}
	}
	if d.Fix && len(stale) > 0 {
		if err := origin.Save(source, origins); err != nil {
			return err
		}
	}

	n, err := d.checkSets(source, skills)
	if err != nil {
		return err
	}
	problems += n
	fixable += n

	for _, b := range target.Builtins() {
		entries, err := target.Scan(b.Path, skills)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if e.State != target.Broken {
				continue
			}
			problems++
			fixable++
			path := filepath.Join(b.Path, e.Name)
			if d.Fix {
				if err := os.Remove(path); err != nil {
					return err
				}
				fmt.Printf("%s: removed broken link %s\n", b.Name, e.Name)
			} else {
				fmt.Printf("%s: broken link %s → %s\n", b.Name, e.Name, e.Dest)
			}
		}
	}

	n, err = checkDrift(source, skills)
	if err != nil {
		return err
	}
	problems += n

	if problems == 0 {
		fmt.Println("all good")
	} else if !d.Fix && fixable > 0 {
		fmt.Println("\nrun `memex doctor --fix` to remove broken links and prune stale entries")
	}
	return nil
}

// checkSets reports set members the library no longer has, dropping them under
// --fix, and returns how many it found.
func (d *doctorCmd) checkSets(source string, skills []library.Skill) (int, error) {
	store, err := project.Load(source)
	if err != nil {
		return 0, err
	}
	if !store.Found {
		return 0, nil
	}
	inLibrary := make(map[string]bool, len(skills))
	for _, s := range skills {
		inLibrary[s.Name] = true
	}
	problems, fixed := 0, false
	for _, name := range append([]string{project.DefaultName}, store.Names()...) {
		var kept []string
		for _, skill := range store.Skills(name) {
			if inLibrary[skill] {
				kept = append(kept, skill)
				continue
			}
			problems++
			if d.Fix {
				fmt.Printf("%s: dropped %s, which is not in the library\n", name, skill)
			} else {
				fmt.Printf("%s: names %s, which is not in the library\n", name, skill)
			}
		}
		if d.Fix && len(kept) != len(store.Skills(name)) {
			store.SetSkills(name, kept)
			fixed = true
		}
	}
	if fixed {
		return problems, project.Save(source, store)
	}
	return problems, nil
}

// checkDrift reports providers that no longer match the set in force — a new
// harness appeared, or someone linked by hand.
func checkDrift(source string, skills []library.Skill) (int, error) {
	store, err := project.Load(source)
	if err != nil {
		return 0, err
	}
	cfg, err := config.Load()
	if err != nil {
		return 0, err
	}
	providers := target.Enabled(cfg.DisabledProviders)
	if len(providers) == 0 {
		return 0, nil
	}
	active, err := project.Active()
	if err != nil {
		return 0, err
	}
	if !store.Has(active) {
		fmt.Printf("state: active project %q is not in %s\n", active, project.File(source))
		return 1, nil
	}
	name := project.DefaultName
	if active != "" {
		name = active
	}
	statuses, err := plan.Survey(providers, skills)
	if err != nil {
		return 0, err
	}
	// Before any set has been stored the set in force is whatever is linked, so
	// drift means the providers disagree with each other.
	want := store.Skills(name)
	if !store.Found {
		want = plan.LinkedNames(statuses)
	}
	p := plan.Build(providers, statuses, want)
	problems := 0
	for _, b := range p.Blocked {
		problems++
		fmt.Printf("%s: %s cannot be linked — %s\n", b.Provider, b.Skill, b.Reason)
	}
	if !p.Empty() {
		problems++
		if store.Found {
			fmt.Printf("providers: %s is not applied everywhere (%s) — run `memex sync`\n", name, p.Summary())
		} else {
			fmt.Printf("providers: they don't all hold the same skills (%s) — run `memex sync` to even them up\n", p.Summary())
		}
	}
	return problems, nil
}

type upgradeCmd struct {
	Check bool `help:"Report whether a newer release exists without installing it."`
}

// Run replaces the running binary with the latest release. It's a no-op when
// already current, and refuses to touch a binary built with `go install`,
// where `go install …@latest` is the right way to update.
func (u *upgradeCmd) Run(c *cli) error {
	current := version()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	src := release.GitHub()
	latest, err := src.Latest(ctx)
	if err != nil {
		return err
	}
	if !release.Newer(current, latest) {
		fmt.Printf("memex %s is already the latest release\n", current)
		return nil
	}
	fmt.Printf("%s → %s\n", current, latest)
	if u.Check {
		fmt.Println("run `memex upgrade` to install it")
		return nil
	}

	self, err := os.Executable()
	if err != nil {
		return err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return err
	}
	if inGoBin(self) {
		return fmt.Errorf("this memex was installed with `go install`; update it with:\n    go install github.com/%s@latest", release.Repo)
	}

	fmt.Printf("downloading %s…\n", release.AssetName(latest, runtime.GOOS, runtime.GOARCH))
	bin, err := src.Binary(ctx, latest, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return err
	}
	if err := release.Replace(self, bin); err != nil {
		return err
	}
	fmt.Printf("upgraded memex to %s\n", latest)
	return nil
}

// inGoBin reports whether path lives under the Go install bin dir, where a
// self-replace would fight with the go toolchain.
func inGoBin(path string) bool {
	if gobin := os.Getenv("GOBIN"); gobin != "" {
		if rel, err := filepath.Rel(gobin, path); err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	if gopath := os.Getenv("GOPATH"); gopath != "" {
		for _, p := range filepath.SplitList(gopath) {
			if rel, err := filepath.Rel(filepath.Join(p, "bin"), path); err == nil && !strings.HasPrefix(rel, "..") {
				return true
			}
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rel, err := filepath.Rel(filepath.Join(home, "go", "bin"), path); err == nil && !strings.HasPrefix(rel, "..") {
			return true
		}
	}
	return false
}
