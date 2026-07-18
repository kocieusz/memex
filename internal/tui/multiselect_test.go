package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func driveMulti(m MultiModel, keys ...string) MultiModel {
	for _, k := range keys {
		next, _ := m.Update(key(k))
		m = next.(MultiModel)
	}
	return m
}

func testItems() []MultiItem {
	return []MultiItem{
		{Name: "alpha", Note: "first"},
		{Name: "beta", Note: "second"},
		{Name: "gamma", Conflict: "already in the library"},
	}
}

func TestMultiToggleAndSelected(t *testing.T) {
	m := NewMultiModel("t", testItems())
	m = driveMulti(m, "space", "j", "space")
	if got := m.Selected(); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("got %v", got)
	}
	m = driveMulti(m, "space")
	if got := m.Selected(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("toggling back must deselect, got %v", got)
	}
}

func TestMultiConflictIsNotToggleable(t *testing.T) {
	m := driveMulti(NewMultiModel("t", testItems()), "j", "j", "space")
	if got := m.Selected(); len(got) != 0 {
		t.Fatalf("conflict row toggled: %v", got)
	}
	all := driveMulti(NewMultiModel("t", testItems()), "a")
	if got := all.Selected(); len(got) != 2 {
		t.Fatalf("`a` must skip conflicts, got %v", got)
	}
	none := driveMulti(all, "n")
	if got := none.Selected(); len(got) != 0 {
		t.Fatalf("`n` must clear, got %v", got)
	}
}

func TestMultiConfirmFlow(t *testing.T) {
	m := driveMulti(NewMultiModel("t", testItems()), "space", "enter")
	if !m.confirming {
		t.Fatal("enter with a selection must ask for confirmation")
	}
	declined := driveMulti(m, "x")
	if declined.Accepted || declined.confirming {
		t.Fatalf("non-y must cancel confirmation: %+v", declined)
	}
	accepted := driveMulti(m, "y")
	if !accepted.Accepted {
		t.Fatal("y must accept the selection")
	}
}

func TestMultiEnterWithoutSelectionQuits(t *testing.T) {
	m := driveMulti(NewMultiModel("t", testItems()), "enter")
	if m.Accepted || m.confirming {
		t.Fatalf("empty enter must just leave: %+v", m)
	}
}

func TestMultiInfoTogglesDescription(t *testing.T) {
	items := []MultiItem{{Name: "alpha", Note: "sub/alpha", Desc: "does alpha things"}}
	m := driveMulti(NewMultiModel("t", items), "i")
	if !strings.Contains(m.View(), "does alpha things") {
		t.Fatalf("i must reveal the description:\n%s", m.View())
	}
	m = driveMulti(m, "i")
	if strings.Contains(m.View(), "does alpha things") {
		t.Fatalf("second i must hide the description:\n%s", m.View())
	}
}

func TestMultiFilterScopesToggles(t *testing.T) {
	m := NewMultiModel("t", testItems())
	m = driveMulti(m, "/", "b", "e", "t")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(MultiModel)
	m = driveMulti(m, "space")
	if got := m.Selected(); len(got) != 1 || got[0] != 1 {
		t.Fatalf("filtered toggle must hit beta, got %v", got)
	}
}
