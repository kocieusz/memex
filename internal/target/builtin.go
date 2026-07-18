package target

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Builtin is a well-known global harness skills directory.
type Builtin struct {
	Name string
	Path string // absolute
}

// Builtins returns the global targets whose harness is installed, i.e. whose
// parent directory (~/.claude, ~/.codex, …) exists. The skills dir itself may
// not exist yet; Link creates it.
func Builtins() []Builtin {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	all := []Builtin{
		{"claude", filepath.Join(home, ".claude", "skills")},
		{"codex", filepath.Join(home, ".codex", "skills")},
		{"pi", filepath.Join(home, ".pi", "agent", "skills")},
		{"agents", filepath.Join(home, ".agents", "skills")},
	}
	var present []Builtin
	for _, b := range all {
		if fi, err := os.Stat(filepath.Dir(b.Path)); err == nil && fi.IsDir() {
			present = append(present, b)
		}
	}
	return present
}

// Resolve turns a --target value (builtin name or filesystem path) into an
// absolute directory path. A leading ~ is expanded, since shells don't expand
// it inside --flag=~/... values.
func Resolve(nameOrPath string) (string, error) {
	for _, b := range Builtins() {
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
	names := make([]string, 0, 4)
	for _, b := range Builtins() {
		names = append(names, b.Name)
	}
	return "", fmt.Errorf("unknown target %q: expected one of %s, or a path", nameOrPath, strings.Join(names, ", "))
}

// BuiltinFor returns the builtin whose path is dir, if any.
func BuiltinFor(dir string) (Builtin, bool) {
	for _, b := range Builtins() {
		if SameDir(dir, b.Path) {
			return b, true
		}
	}
	return Builtin{}, false
}
