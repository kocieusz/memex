package target

import (
	"os"
	"path/filepath"
	"strings"
)

// GitDir walks up from dir and returns the enclosing repository's .git
// directory, if any.
func GitDir(dir string) (string, bool) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	for {
		gd := filepath.Join(dir, ".git")
		if fi, err := os.Stat(gd); err == nil && fi.IsDir() {
			return gd, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// AddIgnore appends "/name" lines to targetDir/.gitignore for names not
// already listed (no trailing slash: git's dir-only patterns don't match
// symlinks). Reports whether the file changed.
func AddIgnore(targetDir string, names []string) (bool, error) {
	file := filepath.Join(targetDir, ".gitignore")
	existing, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	have := map[string]bool{}
	for line := range strings.SplitSeq(string(existing), "\n") {
		have[strings.TrimSpace(line)] = true
	}
	var add []string
	for _, n := range names {
		if !have["/"+n] && !have[n] {
			add = append(add, "/"+n)
		}
	}
	if len(add) == 0 {
		return false, nil
	}
	content := strings.Join(add, "\n") + "\n"
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		content = "\n" + content
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return false, err
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		return false, err
	}
	return true, nil
}

// RemoveIgnore drops the "/name" lines from targetDir/.gitignore, deleting the
// file when only blank lines remain. A missing file or absent lines are a
// no-op. Reports whether the file changed.
func RemoveIgnore(targetDir string, names []string) (bool, error) {
	file := filepath.Join(targetDir, ".gitignore")
	existing, err := os.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	drop := make(map[string]bool, len(names))
	for _, n := range names {
		drop["/"+n] = true
	}
	var keep []string
	changed := false
	rest := true // only blank lines kept so far
	for line := range strings.SplitSeq(strings.TrimRight(string(existing), "\n"), "\n") {
		if drop[strings.TrimSpace(line)] {
			changed = true
			continue
		}
		if strings.TrimSpace(line) != "" {
			rest = false
		}
		keep = append(keep, line)
	}
	if !changed {
		return false, nil
	}
	if rest {
		return true, os.Remove(file)
	}
	return true, os.WriteFile(file, []byte(strings.Join(keep, "\n")+"\n"), 0o644)
}
