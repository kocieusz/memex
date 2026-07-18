package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// Option is one row of the target picker.
type Option struct {
	Label string
	Path  string
}

type pickerModel struct {
	title   string
	options []Option
	cursor  int
	chosen  int // -1 until picked
	// quitting blanks the final frame so bubbletea erases the picker instead
	// of leaving it to stack above the checklist.
	quitting bool
}

func (m pickerModel) Init() tea.Cmd { return nil }

func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
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
		if m.cursor < len(m.options)-1 {
			m.cursor++
		}
	case "enter", " ", "right":
		m.chosen = m.cursor
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m pickerModel) View() string {
	if m.quitting {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "  %s\n\n", headStyle.Render(m.title))
	for i, o := range m.options {
		marker := "  "
		if i == m.cursor {
			marker = cursorStyle.Render("▸ ")
		}
		fmt.Fprintf(&b, "  %s%-8s %s\n", marker, o.Label, dimStyle.Render(o.Path))
	}
	b.WriteString("\n")
	b.WriteString(footer([2]string{"↑/↓", "move"}, [2]string{"enter/→", "select"}, [2]string{"q", "quit"}))
	b.WriteString("\n")
	return b.String()
}

// Pick shows a target picker and returns the chosen option, or ok=false if the
// user quit.
func Pick(title string, options []Option) (Option, bool, error) {
	final, err := tea.NewProgram(pickerModel{title: title, options: options, chosen: -1}).Run()
	if err != nil {
		return Option{}, false, err
	}
	m := final.(pickerModel)
	if m.chosen < 0 {
		return Option{}, false, nil
	}
	return m.options[m.chosen], true, nil
}
