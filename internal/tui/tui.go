// Package tui implements the interactive screens: the checklist that edits one
// skill set, the multi-select used by `memex clone`, and a small list picker.
// The checklist mutates nothing — it returns the set the user confirmed and the
// caller applies it.
package tui

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kocieusz/memex/internal/target"
)

// Blocker is a provider that won't accept a skill, and why.
type Blocker struct {
	Provider string
	Reason   string
}

// Row is one line of the checklist: a library skill, or a name the set holds
// that the library no longer has.
type Row struct {
	Name string
	// InSet is true when the set being edited already names this skill.
	InSet bool
	// LinkedIn names the providers where the skill is linked right now.
	LinkedIn []string
	// Blocked names the providers that can't take it, e.g. because a real
	// directory of the same name is there.
	Blocked []Blocker
	// Missing is true for a name the set holds but the library doesn't have.
	Missing bool
}

type setRow struct {
	row     Row
	checked bool
}

// Model is the checklist screen. Exported for tests.
type Model struct {
	// Project is the name of the set being edited: "default" or a project.
	Project string
	// Source is the library directory, shown in the header.
	Source string
	// Providers names the harnesses an apply would touch.
	Providers []string
	// Live is true when this set is the active one, so what is linked reflects
	// it and applying will move symlinks. Editing an inactive project only
	// changes which skills it names.
	Live bool

	rows       []setRow
	cursor     int // index into visible()
	filter     string
	filtering  bool
	confirming bool
	// quitting blanks the final frame so bubbletea erases the screen's block
	// instead of leaving it to stack under the next program's output.
	quitting bool

	// Accepted is true when the user confirmed the set.
	Accepted bool
}

// NewModel builds the checklist for one set.
func NewModel(project, source string, providers []string, rows []Row) Model {
	list := make([]setRow, len(rows))
	for i, r := range rows {
		list[i] = setRow{row: r, checked: r.InSet}
	}
	return Model{Project: project, Source: source, Providers: providers, rows: list}
}

// Set returns the skill names the user has checked, sorted.
func (m Model) Set() []string {
	var names []string
	for _, r := range m.rows {
		if r.checked {
			names = append(names, r.row.Name)
		}
	}
	sort.Strings(names)
	return names
}

// Dirty reports whether applying would change anything — the set's membership,
// or the symlinks that should already match it.
func (m Model) Dirty() bool {
	for _, r := range m.rows {
		switch {
		case r.checked != r.row.InSet:
			return true
		case !m.Live:
		case r.checked && len(m.pending(r)) > 0:
			return true
		case !r.checked && len(r.row.LinkedIn) > 0:
			return true
		}
	}
	return false
}

// Summary describes what applying would do, for the confirmation line.
func (m Model) Summary() string {
	if !m.Live {
		added, removed := 0, 0
		for _, r := range m.rows {
			switch {
			case r.checked && !r.row.InSet:
				added++
			case !r.checked && r.row.InSet:
				removed++
			}
		}
		return counts(added, "added", removed, "removed") + " in project " + m.Project
	}
	links, unlinks := 0, 0
	for _, r := range m.rows {
		if r.checked {
			links += len(m.pending(r))
		} else {
			unlinks += len(r.row.LinkedIn)
		}
	}
	across := fmt.Sprintf(" across %d providers", len(m.Providers))
	if len(m.Providers) == 1 {
		across = " in " + m.Providers[0]
	}
	return counts(links, "link", unlinks, "unlink") + across
}

func counts(a int, aWord string, b int, bWord string) string {
	var parts []string
	if a > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", a, aWord))
	}
	if b > 0 {
		parts = append(parts, fmt.Sprintf("−%d %s", b, bWord))
	}
	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

// pending returns the providers that would gain a link for this row: everything
// memex manages, minus where it is already linked or can't be.
func (m Model) pending(r setRow) []string {
	if r.row.Missing {
		return nil
	}
	var out []string
	for _, p := range m.Providers {
		if slices.Contains(r.row.LinkedIn, p) || blocks(r.row.Blocked, p) {
			continue
		}
		out = append(out, p)
	}
	return out
}

func blocks(blockers []Blocker, provider string) bool {
	for _, b := range blockers {
		if b.Provider == provider {
			return true
		}
	}
	return false
}

func (m Model) visible() []int {
	var idx []int
	for i, r := range m.rows {
		if m.filter == "" || strings.Contains(r.row.Name, m.filter) {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.confirming {
		switch key.String() {
		case "y", "Y":
			m.Accepted = true
			m.quitting = true
			return m, tea.Quit
		default:
			m.confirming = false
		}
		return m, nil
	}

	if m.filtering {
		switch key.Type {
		case tea.KeyEsc:
			m.filtering, m.filter = false, ""
		case tea.KeyEnter:
			m.filtering = false
		case tea.KeyBackspace:
			if len(m.filter) > 0 {
				m.filter = m.filter[:len(m.filter)-1]
			}
		case tea.KeyRunes:
			m.filter += string(key.Runes)
		}
		m.clampCursor()
		return m, nil
	}

	switch key.String() {
	case "q", "esc", "ctrl+c":
		m.quitting = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.visible())-1 {
			m.cursor++
		}
	case " ":
		if vis := m.visible(); len(vis) > 0 {
			i := vis[m.cursor]
			m.rows[i].checked = !m.rows[i].checked
		}
	case "a":
		m.setAll(true)
	case "n":
		m.setAll(false)
	case "/":
		m.filtering = true
		m.filter = ""
	case "enter":
		if !m.Dirty() {
			m.quitting = true
			return m, tea.Quit
		}
		m.confirming = true
	}
	return m, nil
}

// setAll checks or unchecks every visible row.
func (m *Model) setAll(checked bool) {
	for _, i := range m.visible() {
		m.rows[i].checked = checked
	}
}

func (m *Model) clampCursor() {
	if n := len(m.visible()); m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

var (
	headStyle      = lipgloss.NewStyle().Bold(true)
	dimStyle       = lipgloss.NewStyle().Faint(true)
	brokenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	linkedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	pendingStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	availableStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	cursorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	helpStyle      = lipgloss.NewStyle().Faint(true)
)

var keyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))

// footer renders a key legend like "↑/↓ move · space toggle" with the keys
// tinted so they stand out from their labels.
func footer(items ...[2]string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = keyStyle.Render(it[0]) + " " + helpStyle.Render(it[1])
	}
	return "  " + strings.Join(parts, helpStyle.Render(" · "))
}

// Dim renders s in the faint style, for secondary detail in plain output.
func Dim(s string) string { return dimStyle.Render(s) }

// Tone selects the colour of a badge.
type Tone int

const (
	// Muted is for states memex doesn't act on.
	Muted Tone = iota
	// Good is for something in place: linked, enabled.
	Good
	// Info is for something offered but not in place: available.
	Info
	// Bad is for something wrong: broken, in the way.
	Bad
)

// Badge renders a fixed-width, coloured label for plain CLI output.
func Badge(t Tone, text string) string {
	label := fmt.Sprintf("%-10s", text)
	switch t {
	case Good:
		return linkedStyle.Render(label)
	case Info:
		return availableStyle.Render(label)
	case Bad:
		return brokenStyle.Render(label)
	default:
		return dimStyle.Render(label)
	}
}

// LibraryBadge is the badge for skills listed from the library itself.
func LibraryBadge() string {
	return headStyle.Render(fmt.Sprintf("%-10s", "library"))
}

// StateBadge renders a target entry's state as a badge.
func StateBadge(s target.State) string {
	switch s {
	case target.Linked:
		return Badge(Good, s.String())
	case target.Available:
		return Badge(Info, s.String())
	case target.Broken:
		return Badge(Bad, s.String())
	default:
		return Badge(Muted, s.String())
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	head := headStyle.Render("Project: " + m.Project)
	if !m.Live {
		head += dimStyle.Render(fmt.Sprintf(" (inactive — `memex use %s` applies it)", m.Project))
	}
	fmt.Fprintf(&b, "  %s  %s\n", head, dimStyle.Render("Providers: "+strings.Join(m.Providers, ", ")))
	skills := 0
	for _, r := range m.rows {
		if !r.row.Missing {
			skills++
		}
	}
	fmt.Fprintf(&b, "  %s\n\n", dimStyle.Render(fmt.Sprintf("Source: %s (%d skills)", m.Source, skills)))

	vis := m.visible()
	nameW := 0
	for _, i := range vis {
		nameW = max(nameW, len(m.rows[i].row.Name))
	}
	for pos, i := range vis {
		marker := "  "
		if pos == m.cursor {
			marker = cursorStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "  %s%s\n", marker, m.renderRow(m.rows[i], nameW))
	}
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("  no skills match\n"))
	}

	b.WriteString("\n")
	switch {
	case m.confirming:
		fmt.Fprintf(&b, "  %s — proceed? [y/N] ", m.Summary())
	case m.filtering:
		fmt.Fprintf(&b, "  /%s▏\n", m.filter)
	default:
		apply := "apply"
		if !m.Live {
			apply = "save"
		}
		b.WriteString(footer(
			[2]string{"↑/↓", "move"}, [2]string{"space", "toggle"},
			[2]string{"a", "all"}, [2]string{"n", "none"},
			[2]string{"/", "filter"}, [2]string{"enter", apply},
			[2]string{"q", "quit"},
		))
		b.WriteString("\n")
	}
	return b.String()
}

func (m Model) renderRow(r setRow, nameW int) string {
	name := fmt.Sprintf("%-*s", nameW, r.row.Name)
	row := fmt.Sprintf("%s %s  %s", m.box(r), name, m.note(r))
	if len(r.row.Blocked) > 0 && r.checked {
		var where []string
		for _, blk := range r.row.Blocked {
			where = append(where, blk.Provider)
		}
		row += brokenStyle.Render("  · blocked in " + strings.Join(where, ", "))
	}
	return row
}

func (m Model) box(r setRow) string {
	switch {
	case !r.checked:
		return "[ ]"
	case r.row.Missing:
		return brokenStyle.Render("[!]")
	case m.Live && len(m.pending(r)) > 0:
		return pendingStyle.Render("[~]")
	default:
		return linkedStyle.Render("[x]")
	}
}

func (m Model) note(r setRow) string {
	if r.row.Missing {
		if r.checked {
			return brokenStyle.Render("named by this set but not in the library")
		}
		return pendingStyle.Render("will drop from the set")
	}
	if !m.Live {
		switch {
		case r.checked && !r.row.InSet:
			return pendingStyle.Render("will add")
		case !r.checked && r.row.InSet:
			return pendingStyle.Render("will remove")
		case r.checked:
			return dimStyle.Render("in project")
		default:
			return dimStyle.Render("not in project")
		}
	}
	switch pend := m.pending(r); {
	case r.checked && len(pend) > 0:
		return pendingStyle.Render("will link" + scope(pend, m.Providers, "in"))
	case !r.checked && len(r.row.LinkedIn) > 0:
		return pendingStyle.Render("will unlink" + scope(r.row.LinkedIn, m.Providers, "from"))
	case r.checked:
		return dimStyle.Render("linked" + scope(r.row.LinkedIn, m.Providers, "in"))
	default:
		return dimStyle.Render("available")
	}
}

// scope names the providers a note applies to, staying silent when it applies
// to all of them — "will link" reads better than "will link in claude, codex".
func scope(some, all []string, preposition string) string {
	if len(some) == 0 || len(some) == len(all) {
		return ""
	}
	return " " + preposition + " " + strings.Join(some, ", ")
}

// Run shows the checklist and returns the final model: Accepted reports whether
// the user confirmed, and Set() is the skill set they chose.
func Run(m Model) (Model, error) {
	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return Model{}, err
	}
	return final.(Model), nil
}
