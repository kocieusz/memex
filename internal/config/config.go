// Package config loads the memex config file, a small TOML file at
// ~/.memex/config.toml — alongside the default library, ~/.memex/skills.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config holds the settings read from the config file.
type Config struct {
	// Library is the skill library directory; a leading ~ is expanded.
	Library string `toml:"library"`
	// DisabledProviders names harnesses memex must leave alone even when it
	// detects them, as managed by `memex providers enable|disable`. Everything
	// else that is detected is managed.
	DisabledProviders []string `toml:"disabled_providers"`
}

// Path returns the config file location, ~/.memex/config.toml.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".memex", "config.toml"), nil
}

// Load reads the config file. A missing file is not an error and yields the
// zero Config; a malformed file or an unknown key is.
func Load() (Config, error) {
	path, err := Path()
	if err != nil {
		return Config{}, err
	}
	var c Config
	md, err := toml.DecodeFile(path, &c)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("reading %s: %w", path, err)
	}
	if undecoded := md.Undecoded(); len(undecoded) > 0 {
		return Config{}, fmt.Errorf("%s: unknown key %q", path, undecoded[0].String())
	}
	return c, nil
}

// SetDisabledProviders rewrites the disabled_providers key in the config file,
// leaving every other line — comments included — as it was. An empty list
// removes the key; a missing config file is created.
func SetDisabledProviders(names []string) error {
	path, err := Path()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := ""
	if len(names) > 0 {
		quoted := make([]string, len(names))
		for i, n := range names {
			quoted[i] = strconv.Quote(n)
		}
		line = "disabled_providers = [" + strings.Join(quoted, ", ") + "]"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(setKey(string(data), "disabled_providers", line)), 0o644)
}

// setKey replaces the top-level assignment of key with line, or drops it when
// line is empty, and returns the new file contents. A key that isn't in the
// file yet goes in above the first table header, since anything below one would
// belong to that table.
func setKey(content, key, line string) string {
	var lines []string
	if content != "" {
		lines = strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	}
	assign := regexp.MustCompile(`^\s*` + regexp.QuoteMeta(key) + `\s*=`)
	var out []string
	placed := line == ""
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if !placed && strings.HasPrefix(strings.TrimSpace(l), "[") {
			out = append(out, line)
			placed = true
		}
		if assign.MatchString(l) {
			if !placed {
				out = append(out, line)
				placed = true
			}
			// Skip the old assignment, including an array value spread over
			// several lines.
			if _, value, _ := strings.Cut(l, "="); strings.Contains(value, "[") && !strings.Contains(value, "]") {
				for i+1 < len(lines) && !strings.Contains(lines[i], "]") {
					i++
				}
			}
			continue
		}
		out = append(out, l)
	}
	if !placed {
		out = append(out, line)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}
