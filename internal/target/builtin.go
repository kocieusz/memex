package target

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Builtin is a well-known global harness skills directory — a provider, in the
// user-facing vocabulary.
type Builtin struct {
	Name string
	Path string // absolute
}

// All returns every provider memex knows how to manage, installed or not.
func All() []Builtin {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []Builtin{
		{"claude", filepath.Join(home, ".claude", "skills")},
		{"codex", filepath.Join(home, ".codex", "skills")},
		{"pi", filepath.Join(home, ".pi", "agent", "skills")},
		{"agents", filepath.Join(home, ".agents", "skills")},
	}
}

// Present reports whether the harness is installed, i.e. whether its parent
// directory (~/.claude, ~/.codex, …) exists. The skills dir itself may not
// exist yet; Link creates it.
func (b Builtin) Present() bool {
	fi, err := os.Stat(filepath.Dir(b.Path))
	return err == nil && fi.IsDir()
}

// Builtins returns the providers detected on this machine.
func Builtins() []Builtin {
	var present []Builtin
	for _, b := range All() {
		if b.Present() {
			present = append(present, b)
		}
	}
	return present
}

// Enabled returns the detected providers memex manages: everything it detects
// except the names disabled in the config file.
func Enabled(disabled []string) []Builtin {
	var out []Builtin
	for _, b := range Builtins() {
		if !slices.Contains(disabled, b.Name) {
			out = append(out, b)
		}
	}
	return out
}

// Supported reports whether name is a provider memex knows.
func Supported(name string) bool {
	for _, b := range All() {
		if b.Name == name {
			return true
		}
	}
	return false
}

// Names lists the provider names of builtins, for messages.
func Names(builtins []Builtin) []string {
	names := make([]string, len(builtins))
	for i, b := range builtins {
		names[i] = b.Name
	}
	return names
}

// Resolve turns a --target value (builtin name or filesystem path) into an
// absolute directory path. A leading ~ is expanded, since shells don't expand
// it inside --flag=~/... values.
func Resolve(nameOrPath string) (string, error) {
	for _, b := range All() {
		if nameOrPath == b.Name {
			return b.Path, nil
		}
	}
	if nameOrPath == "~" || strings.HasPrefix(nameOrPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, strings.TrimPrefix(nameOrPath[1:], "/")), nil
	}
	if strings.ContainsRune(nameOrPath, os.PathSeparator) || nameOrPath == "." || nameOrPath == ".." {
		return filepath.Abs(nameOrPath)
	}
	return "", fmt.Errorf("unknown target %q: expected one of %s, or a path", nameOrPath, strings.Join(Names(All()), ", "))
}
