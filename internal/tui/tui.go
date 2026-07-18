// Package tui implements the interactive screens: the skill checklist for one
// target directory, and the target picker. The checklist mutates nothing — it
// returns a Plan that the caller applies.
package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/kocieusz/memex/internal/target"
)

// Plan is the outcome of the checklist: skill names to link and unlink.
type Plan struct {
	Link   []string
	Unlink []string
}

func (p Plan) Empty() bool { return len(p.Link) == 0 && len(p.Unlink) == 0 }

func (p Plan) Summary() string {
	var parts []string
	if n := len(p.Link); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d link", n))
	}
	if n := len(p.Unlink); n > 0 {
		parts = append(parts, fmt.Sprintf("−%d unlink", n))
	}
	return strings.Join(parts, ", ")
}

type row struct {
	entry   target.Entry
	pending bool // toggled away from its current state
}

// Model is the checklist screen. Exported for tests.
type Model struct {
	TargetDir string
	Source    string
	// Back is true when quitting returns to a picker rather than the shell.
	Back bool

	rows       []row
	cursor     int // index into visible()
	filter     string
	filtering  bool
	confirming bool
	// quitting blanks the final frame so bubbletea erases the screen's block
	// instead of leaving it to stack under the next program's output.
	quitting bool

	// Accepted is true when the user confirmed the plan.
	Accepted bool
	// Quit is true when the user asked to leave the session (q/ctrl+c)
	// rather than return to the picker (esc/left).
	Quit bool
}

// NewModel builds the checklist for entries as returned by target.Scan.
func NewModel(targetDir, source string, entries []target.Entry) Model {
	rows := make([]row, len(entries))
	for i, e := range entries {
		rows[i] = row{entry: e}
	}
	return Model{TargetDir: targetDir, Source: source, rows: rows}
}

// Plan returns the pending changes.
func (m Model) Plan() Plan {
	var p Plan
	for _, r := range m.rows {
		if !r.pending {
			continue
		}
		switch r.entry.State {
		case target.Available:
			p.Link = append(p.Link, r.entry.Name)
		case target.Linked:
			p.Unlink = append(p.Unlink, r.entry.Name)
		}
	}
	return p
}

func (m Model) visible() []int {
	var idx []int
	for i, r := range m.rows {
		if m.filter == "" || strings.Contains(r.entry.Name, m.filter) {
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
	case "q", "ctrl+c":
		m.quitting = true
		m.Quit = true
		return m, tea.Quit
	case "esc", "left":
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
			if m.rows[i].entry.Toggleable() {
				m.rows[i].pending = !m.rows[i].pending
			}
		}
	case "a":
		m.setAll(true)
	case "n":
		m.setAll(false)
	case "/":
		m.filtering = true
		m.filter = ""
	case "enter":
		if m.Plan().Empty() {
			m.quitting = true
			return m, tea.Quit
		}
		m.confirming = true
	}
	return m, nil
}

// setAll pends every toggleable row toward linked (true) or unlinked (false).
func (m *Model) setAll(linked bool) {
	for _, i := range m.visible() {
		r := &m.rows[i]
		switch r.entry.State {
		case target.Available:
			r.pending = linked
		case target.Linked:
			r.pending = !linked
		}
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

// LibraryBadge is the badge for skills listed from the library itself.
func LibraryBadge() string {
	return headStyle.Render(fmt.Sprintf("%-10s", "library"))
}

// StateBadge renders a fixed-width, colored state label for plain CLI output.
func StateBadge(s target.State) string {
	label := fmt.Sprintf("%-10s", s)
	switch s {
	case target.Linked:
		return linkedStyle.Render(label)
	case target.Available:
		return availableStyle.Render(label)
	case target.Broken:
		return brokenStyle.Render(label)
	default:
		return dimStyle.Render(label)
	}
}

func (m Model) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s  %s\n\n",
		headStyle.Render("Target: "+m.TargetDir),
		dimStyle.Render("Source: "+m.Source))

	vis := m.visible()
	nameW := 0
	for _, i := range vis {
		nameW = max(nameW, len(m.rows[i].entry.Name))
	}
	for pos, i := range vis {
		r := m.rows[i]
		marker := "  "
		if pos == m.cursor {
			marker = cursorStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "  %s%s\n", marker, renderRow(r, nameW))
	}
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("  no skills match\n"))
	}

	b.WriteString("\n")
	switch {
	case m.confirming:
		fmt.Fprintf(&b, "  %s — proceed? [y/N] ", m.Plan().Summary())
	case m.filtering:
		fmt.Fprintf(&b, "  /%s▏\n", m.filter)
	default:
		items := [][2]string{
			{"↑/↓", "move"}, {"space", "toggle"},
			{"a", "all"}, {"n", "none"},
			{"/", "filter"}, {"enter", "apply"},
		}
		if m.Back {
			items = append(items, [2]string{"←", "back"})
		}
		items = append(items, [2]string{"q", "quit"})
		b.WriteString(footer(items...))
		b.WriteString("\n")
	}
	return b.String()
}

func renderRow(r row, nameW int) string {
	e := r.entry
	name := fmt.Sprintf("%-*s", nameW, e.Name)
	linked := e.State == target.Linked
	if r.pending {
		linked = !linked
	}
	switch e.State {
	case target.Linked, target.Available:
		box := "[ ]"
		if linked {
			box = linkedStyle.Render("[x]")
		}
		note := dimStyle.Render(e.State.String())
		if r.pending {
			note = pendingStyle.Render(map[bool]string{true: "will link", false: "will unlink"}[linked])
		}
		return fmt.Sprintf("%s %s  %s", box, name, note)
	case target.Native:
		note := "native — not managed by memex"
		if e.Shadows {
			note += " (shadows a library skill)"
		}
		return dimStyle.Render(fmt.Sprintf("( ) %s  %s", name, note))
	case target.Foreign:
		return dimStyle.Render(fmt.Sprintf("( ) %s  foreign link → %s", name, e.Dest))
	case target.Broken:
		return brokenStyle.Render(fmt.Sprintf("(!) %s  broken link → %s", name, e.Dest))
	}
	return name
}

// Run shows the checklist and returns the confirmed plan (empty if the user
// left or confirmed nothing). quit is true when the user asked to leave the
// session rather than return to the picker. back shows the ← back hint.
func Run(targetDir, source string, entries []target.Entry, back bool) (Plan, bool, error) {
	model := NewModel(targetDir, source, entries)
	model.Back = back
	final, err := tea.NewProgram(model).Run()
	if err != nil {
		return Plan{}, false, err
	}
	m := final.(Model)
	if !m.Accepted {
		return Plan{}, m.Quit, nil
	}
	return m.Plan(), m.Quit, nil
}
