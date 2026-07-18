package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/kocieusz/memex/internal/target"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "left":
		return tea.KeyMsg{Type: tea.KeyLeft}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

func drive(m Model, keys ...string) Model {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(Model)
	}
	return m
}

func testEntries() []target.Entry {
	return []target.Entry{
		{Name: "alpha", State: target.Linked},
		{Name: "beta", State: target.Available},
		{Name: "gamma", State: target.Native},
	}
}

func TestToggleBuildsPlan(t *testing.T) {
	m := NewModel("/t", "/s", testEntries())
	// unlink alpha, link beta
	m = drive(m, "space", "j", "space")
	p := m.Plan()
	if len(p.Link) != 1 || p.Link[0] != "beta" || len(p.Unlink) != 1 || p.Unlink[0] != "alpha" {
		t.Fatalf("got %+v", p)
	}
	// toggling back empties the plan
	m = drive(m, "space", "k", "space")
	if !m.Plan().Empty() {
		t.Fatalf("plan should be empty, got %+v", m.Plan())
	}
}

func TestNativeIsNotToggleable(t *testing.T) {
	m := NewModel("/t", "/s", testEntries())
	m = drive(m, "j", "j", "space") // cursor on gamma (native)
	if !m.Plan().Empty() {
		t.Fatalf("native toggled: %+v", m.Plan())
	}
}

func TestConfirmFlow(t *testing.T) {
	m := NewModel("/t", "/s", testEntries())
	m = drive(m, "space", "enter")
	if !m.confirming {
		t.Fatal("enter with changes must ask for confirmation")
	}
	declined := drive(m, "x")
	if declined.Accepted || declined.confirming {
		t.Fatalf("non-y must cancel confirmation: %+v", declined)
	}
	accepted := drive(m, "y")
	if !accepted.Accepted {
		t.Fatal("y must accept the plan")
	}
}

func TestAllAndNone(t *testing.T) {
	m := NewModel("/t", "/s", testEntries())
	all := drive(m, "a").Plan()
	if len(all.Link) != 1 || len(all.Unlink) != 0 {
		t.Fatalf("`a` should link beta only, got %+v", all)
	}
	none := drive(m, "n").Plan()
	if len(none.Unlink) != 1 || len(none.Link) != 0 {
		t.Fatalf("`n` should unlink alpha only, got %+v", none)
	}
}

func TestQuitVersusBack(t *testing.T) {
	quit := drive(NewModel("/t", "/s", testEntries()), "q")
	if !quit.Quit {
		t.Fatal("q must quit the session")
	}
	back := drive(NewModel("/t", "/s", testEntries()), "left")
	if back.Quit {
		t.Fatal("left must return to the picker, not quit")
	}
}

func TestFilterScopesToggles(t *testing.T) {
	m := NewModel("/t", "/s", testEntries())
	m = drive(m, "/", "b", "e", "t") // filter "bet" → only beta visible
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(m, "space")
	p := m.Plan()
	if len(p.Link) != 1 || p.Link[0] != "beta" {
		t.Fatalf("filtered toggle must hit beta, got %+v", p)
	}
}
