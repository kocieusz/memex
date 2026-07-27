package tui

import (
	"slices"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func key(s string) tea.KeyMsg {
	switch s {
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace, Runes: []rune{' '}}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
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

var providers = []string{"claude", "codex"}

// applied is a set whose skills are linked in every provider already.
func applied() Model {
	m := NewModel("default", "~/.memex/skills", providers, []Row{
		{Name: "alpha", InSet: true, LinkedIn: providers},
		{Name: "beta"},
	})
	m.Live = true
	return m
}

func TestToggleBuildsSet(t *testing.T) {
	m := drive(applied(), "space", "j", "space") // drop alpha, add beta
	if got := m.Set(); !slices.Equal(got, []string{"beta"}) {
		t.Fatalf("Set() = %v, want [beta]", got)
	}
	// toggling back restores the stored set and leaves nothing to do
	m = drive(m, "space", "k", "space")
	if got := m.Set(); !slices.Equal(got, []string{"alpha"}) {
		t.Fatalf("Set() = %v, want [alpha]", got)
	}
	if m.Dirty() {
		t.Fatal("returning to the stored set must not be dirty")
	}
}

func TestAppliedSetIsNotDirty(t *testing.T) {
	m := applied()
	if m.Dirty() {
		t.Fatal("a set already linked everywhere must not be dirty")
	}
	// enter with nothing to do leaves without asking to confirm
	m = drive(m, "enter")
	if m.confirming || m.Accepted {
		t.Fatalf("enter must be a no-op: confirming=%v accepted=%v", m.confirming, m.Accepted)
	}
}

// A set member that is linked in only some providers is drift: applying it has
// work to do even though the membership hasn't changed.
func TestPartiallyLinkedSetIsDirty(t *testing.T) {
	m := NewModel("default", "/s", providers, []Row{
		{Name: "alpha", InSet: true, LinkedIn: []string{"claude"}},
	})
	m.Live = true
	if !m.Dirty() {
		t.Fatal("a skill missing from one provider must be dirty")
	}
	if got := m.Summary(); got != "+1 link across 2 providers" {
		t.Errorf("Summary() = %q", got)
	}
}

// A provider that can't take a skill (native dir in the way) is not counted as
// pending work, or every apply would look dirty forever.
func TestBlockedProviderIsNotPending(t *testing.T) {
	m := NewModel("default", "/s", providers, []Row{
		{Name: "alpha", InSet: true, LinkedIn: []string{"claude"},
			Blocked: []Blocker{{Provider: "codex", Reason: "a real directory is in the way"}}},
	})
	m.Live = true
	if m.Dirty() {
		t.Fatal("a blocked provider must not make the set dirty")
	}
}

// An unchecked skill that is still linked has to be unlinked.
func TestStrayLinkIsDirty(t *testing.T) {
	m := NewModel("default", "/s", providers, []Row{
		{Name: "alpha", LinkedIn: providers},
	})
	m.Live = true
	if !m.Dirty() {
		t.Fatal("a linked skill outside the set must be dirty")
	}
	if got := m.Summary(); got != "−2 unlink across 2 providers" {
		t.Errorf("Summary() = %q", got)
	}
}

func TestConfirmFlow(t *testing.T) {
	m := drive(applied(), "space")
	m = drive(m, "enter")
	if !m.confirming {
		t.Fatal("enter with changes must ask for confirmation")
	}
	if declined := drive(m, "x"); declined.Accepted || declined.confirming {
		t.Fatalf("non-y must cancel confirmation: %+v", declined)
	}
	if accepted := drive(m, "y"); !accepted.Accepted {
		t.Fatal("y must accept the set")
	}
}

func TestAllAndNone(t *testing.T) {
	if got := drive(applied(), "a").Set(); !slices.Equal(got, []string{"alpha", "beta"}) {
		t.Errorf("`a` should check everything, got %v", got)
	}
	if got := drive(applied(), "n").Set(); len(got) != 0 {
		t.Errorf("`n` should clear the set, got %v", got)
	}
}

func TestFilterScopesToggles(t *testing.T) {
	m := drive(applied(), "/", "b", "e", "t") // filter "bet" → only beta visible
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(next.(Model), "a") // `a` applies to the filtered rows only
	if got := m.Set(); !slices.Equal(got, []string{"alpha", "beta"}) {
		t.Fatalf("Set() = %v, want alpha kept and beta added", got)
	}
}

// Editing a project that isn't in force only changes membership, so what is
// linked right now is irrelevant.
func TestInactiveProjectIgnoresLinks(t *testing.T) {
	rows := []Row{{Name: "alpha", InSet: true}, {Name: "beta", LinkedIn: providers}}
	m := NewModel("web", "/s", providers, rows)
	if m.Dirty() {
		t.Fatal("an untouched inactive project must not be dirty")
	}
	m = drive(m, "j", "space")
	if !m.Dirty() {
		t.Fatal("adding a skill must be dirty")
	}
	if got := m.Summary(); got != "+1 added in project web" {
		t.Errorf("Summary() = %q", got)
	}
}

// A set can name a skill the library no longer has; unchecking it is how you
// drop it.
func TestMissingSkillCanBeDropped(t *testing.T) {
	m := NewModel("default", "/s", providers, []Row{{Name: "ghost", InSet: true, Missing: true}})
	m.Live = true
	if m.Dirty() {
		t.Fatal("a missing skill is not work to do on its own")
	}
	m = drive(m, "space")
	if got := m.Set(); len(got) != 0 {
		t.Fatalf("Set() = %v, want it dropped", got)
	}
	if !m.Dirty() {
		t.Fatal("dropping a missing skill changes the set")
	}
}
