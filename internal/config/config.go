// Package config loads the memex config file, a small TOML file at
// $XDG_CONFIG_HOME/memex/config.toml (~/.config/memex/config.toml by default).
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds the settings read from the config file.
type Config struct {
	// Library is the skill library directory; a leading ~ is expanded.
	Library string `toml:"library"`
}

// Path returns the config file location, honouring XDG_CONFIG_HOME.
func Path() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "memex", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "memex", "config.toml"), nil
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
