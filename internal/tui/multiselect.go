package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// MultiItem is one row of the multi-select list.
type MultiItem struct {
	Name string
	Note string // dimmed detail shown after the name
	Desc string // longer text shown under the cursor row while info is on
	// Conflict marks the row unselectable, with a reason shown in place of
	// the note.
	Conflict string
}

type multiRow struct {
	item    MultiItem
	checked bool
}

// MultiModel is the multi-select list screen. Exported for tests.
type MultiModel struct {
	Title string

	rows       []multiRow
	cursor     int // index into visible()
	width      int // terminal width, 0 until the first WindowSizeMsg
	filter     string
	filtering  bool
	confirming bool
	showInfo   bool
	// quitting blanks the final frame so bubbletea erases the screen's block
	// instead of leaving it to stack under the next program's output.
	quitting bool

	// Accepted is true when the user confirmed the selection.
	Accepted bool
}

// NewMultiModel builds the multi-select list for items.
func NewMultiModel(title string, items []MultiItem) MultiModel {
	rows := make([]multiRow, len(items))
	for i, it := range items {
		rows[i] = multiRow{item: it}
	}
	return MultiModel{Title: title, rows: rows}
}

// Selected returns the indices of the checked items, in input order.
func (m MultiModel) Selected() []int {
	var idx []int
	for i, r := range m.rows {
		if r.checked {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m MultiModel) visible() []int {
	var idx []int
	for i, r := range m.rows {
		if m.filter == "" || strings.Contains(r.item.Name, m.filter) {
			idx = append(idx, i)
		}
	}
	return idx
}

func (m MultiModel) Init() tea.Cmd { return nil }

func (m MultiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if ws, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = ws.Width
		return m, nil
	}
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
	case "q", "ctrl+c", "esc":
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
			if m.rows[i].item.Conflict == "" {
				m.rows[i].checked = !m.rows[i].checked
			}
		}
	case "a":
		m.setAll(true)
	case "n":
		m.setAll(false)
	case "i":
		m.showInfo = !m.showInfo
	case "/":
		m.filtering = true
		m.filter = ""
	case "enter":
		if len(m.Selected()) == 0 {
			m.quitting = true
			return m, tea.Quit
		}
		m.confirming = true
	}
	return m, nil
}

func (m *MultiModel) setAll(checked bool) {
	for _, i := range m.visible() {
		if m.rows[i].item.Conflict == "" {
			m.rows[i].checked = checked
		}
	}
}

func (m *MultiModel) clampCursor() {
	if n := len(m.visible()); m.cursor >= n {
		m.cursor = max(0, n-1)
	}
}

func (m MultiModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s\n\n", headStyle.Render(m.Title))

	vis := m.visible()
	nameW := 0
	for _, i := range vis {
		nameW = max(nameW, len(m.rows[i].item.Name))
	}
	for pos, i := range vis {
		r := m.rows[i]
		marker := "  "
		if pos == m.cursor {
			marker = cursorStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "  %s%s\n", marker, renderMultiRow(r, nameW))
		if m.showInfo && pos == m.cursor {
			desc := r.item.Desc
			if desc == "" {
				desc = "(no description)"
			}
			width := m.width - 10
			if width < 20 {
				width = 70
			}
			for line := range strings.SplitSeq(dimStyle.Width(width).Render(desc), "\n") {
				fmt.Fprintf(&b, "        %s\n", line)
			}
		}
	}
	if len(vis) == 0 {
		b.WriteString(dimStyle.Render("  no skills match\n"))
	}

	b.WriteString("\n")
	switch {
	case m.confirming:
		fmt.Fprintf(&b, "  copy %d skill(s) into the library — proceed? [y/N] ", len(m.Selected()))
	case m.filtering:
		fmt.Fprintf(&b, "  /%s▏\n", m.filter)
	default:
		b.WriteString(footer(
			[2]string{"↑/↓", "move"}, [2]string{"space", "toggle"},
			[2]string{"a", "all"}, [2]string{"n", "none"},
			[2]string{"i", "info"}, [2]string{"/", "filter"},
			[2]string{"enter", "apply"}, [2]string{"q", "quit"},
		))
		b.WriteString("\n")
	}
	return b.String()
}

func renderMultiRow(r multiRow, nameW int) string {
	name := fmt.Sprintf("%-*s", nameW, r.item.Name)
	if r.item.Conflict != "" {
		return dimStyle.Render(fmt.Sprintf("( ) %s  %s", name, r.item.Conflict))
	}
	box := "[ ]"
	if r.checked {
		box = linkedStyle.Render("[x]")
	}
	return fmt.Sprintf("%s %s  %s", box, name, dimStyle.Render(r.item.Note))
}

// MultiSelect shows the list and returns the indices of the confirmed items;
// ok is false when the user backed out or confirmed nothing.
func MultiSelect(title string, items []MultiItem) ([]int, bool, error) {
	final, err := tea.NewProgram(NewMultiModel(title, items)).Run()
	if err != nil {
		return nil, false, err
	}
	m := final.(MultiModel)
	if !m.Accepted {
		return nil, false, nil
	}
	return m.Selected(), true, nil
}
