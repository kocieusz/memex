package target

import (
	"os"
	"path/filepath"
	"testing"
)

func readIgnore(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestAddIgnoreCreatesAndSkipsDuplicates(t *testing.T) {
	dir := t.TempDir()
	changed, err := AddIgnore(dir, []string{"alpha", "beta"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got, want := readIgnore(t, dir), "/alpha\n/beta\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	changed, err = AddIgnore(dir, []string{"alpha"})
	if err != nil || changed {
		t.Fatalf("re-adding must be a no-op, changed=%v err=%v", changed, err)
	}
}

func TestAddIgnoreAppendsToExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\nalpha"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := AddIgnore(dir, []string{"alpha", "beta"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	// alpha is already covered by a hand-written line; only beta is added,
	// after a newline terminating the existing content.
	if got, want := readIgnore(t, dir), "*.log\nalpha\n/beta\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRemoveIgnoreDropsLinesAndDeletesEmptyFile(t *testing.T) {
	dir := t.TempDir()
	if _, err := AddIgnore(dir, []string{"alpha", "beta"}); err != nil {
		t.Fatal(err)
	}
	changed, err := RemoveIgnore(dir, []string{"alpha"})
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got, want := readIgnore(t, dir), "/beta\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := RemoveIgnore(dir, []string{"beta"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".gitignore")); !os.IsNotExist(err) {
		t.Fatal("emptied .gitignore must be deleted")
	}
}

func TestRemoveIgnoreKeepsUserContent(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := AddIgnore(dir, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if _, err := RemoveIgnore(dir, []string{"alpha"}); err != nil {
		t.Fatal(err)
	}
	if got, want := readIgnore(t, dir), "*.log\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRemoveIgnoreMissingFileIsNoop(t *testing.T) {
	changed, err := RemoveIgnore(t.TempDir(), []string{"alpha"})
	if err != nil || changed {
		t.Fatalf("missing file must be a no-op, changed=%v err=%v", changed, err)
	}
}
