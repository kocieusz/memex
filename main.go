// memex links Agent Skills from a git-versioned library (~/.memex/skills by
// default) into harness skills directories via symlinks.
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/charmbracelet/lipgloss"

	"github.com/kocieusz/memex/internal/config"
	"github.com/kocieusz/memex/internal/library"
	"github.com/kocieusz/memex/internal/remote"
	"github.com/kocieusz/memex/internal/target"
	"github.com/kocieusz/memex/internal/tui"
)

type cli struct {
	Source  string           `help:"Skill library directory (overrides the config file; default ~/.memex/skills)." env:"MEMEX_SOURCE" placeholder:"DIR"`
	Version kong.VersionFlag `help:"Print version and exit."`

	Manage manageCmd `cmd:"" default:"1" hidden:""`
	Global globalCmd `cmd:"" help:"Manage the global harness targets (~/.claude, ~/.codex, …)."`
	Ls     listCmd   `cmd:"" aliases:"list" help:"List skill states for a target."`
	Adopt  adoptCmd  `cmd:"" help:"Move a real skill directory into the library and leave a symlink."`
	Clone  cloneCmd  `cmd:"" help:"Pick skills from a git repo and copy them into the library."`
	Touch  newCmd    `cmd:"" aliases:"new" help:"Scaffold a new skill in the library."`
	Doctor doctorCmd `cmd:"" help:"Check targets and library for problems."`
}

func main() {
	var c cli
	ctx := kong.Parse(&c,
		kong.Name("memex"),
		kong.Description("Manage Agent Skills across harnesses with symlinks from one library."),
		kong.UsageOnError(),
		kong.Help(colorHelp),
		kong.Vars{"version": version()},
	)
	ctx.FatalIfErrorf(ctx.Run(&c))
}

// version reports the module version stamped into `go install` builds; local
// source builds show (devel).
func version() string {
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
			name, rest, _ := strings.Cut(line[2:], " ")
			fmt.Fprintln(out, "  "+cmdStyle.Render(name)+" "+rest)
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

// manage runs the checklist TUI for targetDir and applies the confirmed plan.
// quit is true when the user asked to leave the session rather than return to
// the picker. back shows the ← back hint.
func (c *cli) manage(targetDir string, back bool) (bool, error) {
	source, skills, entries, err := c.scan(targetDir)
	if err != nil {
		return false, err
	}
	plan, quit, err := tui.Run(targetDir, source, entries, back)
	if err != nil {
		return quit, err
	}
	if plan.Empty() {
		if !back {
			fmt.Println("no changes")
		}
		return quit, nil
	}
	return quit, applyPlan(targetDir, source, skills, plan)
}

func applyPlan(targetDir, source string, skills []library.Skill, plan tui.Plan) error {
	byName := map[string]library.Skill{}
	for _, s := range skills {
		byName[s.Name] = s
	}
	for _, name := range plan.Link {
		if err := target.Link(targetDir, byName[name]); err != nil {
			return err
		}
		fmt.Printf("linked   %s\n", name)
	}
	for _, name := range plan.Unlink {
		if err := target.Unlink(targetDir, source, name); err != nil {
			return err
		}
		fmt.Printf("unlinked %s\n", name)
	}
	updateIgnore(targetDir, plan)
	return nil
}

// updateIgnore keeps targetDir/.gitignore in sync with the plan when the
// target sits inside a project repo: the symlinks point into this machine's
// home dir and would be broken for anyone else cloning the project.
func updateIgnore(targetDir string, plan tui.Plan) {
	if _, ok := target.GitDir(targetDir); !ok {
		return
	}
	file := filepath.Join(targetDir, ".gitignore")
	added, err := target.AddIgnore(targetDir, plan.Link)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to update %s: %v\n", file, err)
	}
	removed, err := target.RemoveIgnore(targetDir, plan.Unlink)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to update %s: %v\n", file, err)
	}
	if added || removed {
		fmt.Printf("updated  %s\n", file)
	}
}

func confirm(prompt string) bool {
	fmt.Printf("%s [y/N] ", prompt)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

type manageCmd struct{}

// Run picks a target for the bare `memex` invocation: the CWD when it is a
// skills dir, a harness dir found under the CWD, or an interactive picker.
func (m *manageCmd) Run(c *cli) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	// The library itself is not a target — fall through to the picker.
	if source, err := c.sourceDir(); err == nil && target.SameDir(cwd, source) {
		return pickLoop(c, nil)
	}
	if isSkillsDir(cwd) {
		_, err := c.manage(cwd, false)
		return err
	}

	options := harnessDirsUnder(cwd)
	if len(options) == 1 {
		_, err := c.manage(options[0].Path, false)
		return err
	}
	return pickLoop(c, options)
}

type globalCmd struct{}

// Run manages the global harness targets even when the CWD has project
// skills dirs that bare `memex` would prefer.
func (g *globalCmd) Run(c *cli) error {
	return pickLoop(c, nil)
}

// pickLoop offers options (falling back to the global targets) and manages the
// chosen one, returning to the picker afterwards so several targets can be
// handled in one session.
func pickLoop(c *cli, options []tui.Option) error {
	if len(options) == 0 {
		for _, b := range target.Builtins() {
			options = append(options, tui.Option{Label: b.Name, Path: b.Path})
		}
		if len(options) == 0 {
			return fmt.Errorf("no skills directory found here and no global targets present")
		}
	}
	for {
		opt, ok, err := tui.Pick("Pick a target", options)
		if err != nil || !ok {
			return err
		}
		quit, err := c.manage(opt.Path, true)
		if err != nil || quit {
			return err
		}
	}
}

func isSkillsDir(dir string) bool {
	if filepath.Base(dir) == "skills" {
		return true
	}
	_, ok := target.BuiltinFor(dir)
	return ok
}

// harnessDirsUnder returns the harness skills dirs that exist under dir.
func harnessDirsUnder(dir string) []tui.Option {
	var options []tui.Option
	for _, sub := range []string{".claude/skills", ".codex/skills", ".agents/skills"} {
		path := filepath.Join(dir, sub)
		if fi, err := os.Stat(path); err == nil && fi.IsDir() {
			options = append(options, tui.Option{Label: sub, Path: path})
		}
	}
	return options
}

type listCmd struct {
	Target string `help:"Target name or path (default: detected from the current directory)."`
	All    bool   `short:"a" help:"Also list entries not managed by memex (native dirs, foreign links)."`
	JSON   bool   `help:"Emit JSON."`
}

// listTarget resolves the directory to list: an explicit --target, the CWD
// when it is a skills dir, or the single harness dir under the CWD.
func (l *listCmd) listTarget() (string, error) {
	if l.Target != "" {
		return target.Resolve(l.Target)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if isSkillsDir(cwd) {
		return cwd, nil
	}
	if options := harnessDirsUnder(cwd); len(options) == 1 {
		return options[0].Path, nil
	}
	return "", fmt.Errorf("%s is not a skills directory — pass --target (a name like claude, or a path)", cwd)
}

func (l *listCmd) Run(c *cli) error {
	dir, err := l.listTarget()
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

// listLibrary lists the library's own skills — the degenerate case where the
// target is the library itself.
func (l *listCmd) listLibrary(source string) error {
	skills, err := library.Scan(source)
	if err != nil {
		return err
	}
	if l.JSON {
		type jsonSkill struct {
			Name  string `json:"name"`
			State string `json:"state"`
		}
		out := make([]jsonSkill, len(skills))
		for i, s := range skills {
			out[i] = jsonSkill{s.Name, "library"}
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}
	for _, s := range skills {
		fmt.Printf("%s %s\n", tui.LibraryBadge(), s.Name)
	}
	return nil
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
	inLib := make(map[string]bool, len(skills))
	for _, s := range skills {
		inLib[s.Name] = true
	}
	items := make([]tui.MultiItem, len(found))
	seen := map[string]string{} // name → rel of the first occurrence in the repo
	for i, f := range found {
		items[i] = tui.MultiItem{Name: f.Name, Note: f.Rel, Desc: f.Description}
		switch {
		case inLib[f.Name]:
			items[i].Conflict = "already in the library"
		case seen[f.Name] != "":
			items[i].Conflict = "duplicate name — see " + seen[f.Name]
		default:
			seen[f.Name] = f.Rel
		}
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
		if err := remote.Copy(found[i], source); err != nil {
			return err
		}
		fmt.Printf("copied   %s\n", found[i].Name)
	}
	fmt.Println("run `memex` to link them into a harness")
	return nil
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
	problems := 0

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
	if problems == 0 {
		fmt.Println("all good")
	} else if !d.Fix {
		fmt.Println("\nrun `memex doctor --fix` to remove broken links")
	}
	return nil
}
