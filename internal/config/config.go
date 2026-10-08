// Package config loads the optional user config from
// %USERPROFILE%\.config\revue\config.toml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config is the user-tunable part of the viewer.
type Config struct {
	Context int    `toml:"context"` // lines of context around changes
	Depth   int    `toml:"depth"`   // repo discovery depth below the launch folder
	Editor  string `toml:"editor"`  // opened as: editor +LINE FILE
	Claude  Claude `toml:"claude"`
}

// Claude configures how questions are sent to the claude CLI.
type Claude struct {
	Model     string   `toml:"model"`
	Effort    string   `toml:"effort"`
	ExtraArgs []string `toml:"extra_args"`
}

// Default is used for every field the config file leaves out.
func Default() Config {
	return Config{
		Context: 50,
		Depth:   2,
		Editor:  "nvim",
		Claude:  Claude{Model: "sonnet", Effort: "medium"},
	}
}

// Path is where the config file lives.
func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "revue", "config.toml"), nil
}

// Load reads the config file on top of the defaults. A missing file is not an
// error.
func Load() (Config, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return cfg, fmt.Errorf("config %s: %w", path, err)
	}
	if cfg.Context < 0 {
		cfg.Context = 0
	}
	if cfg.Depth < 1 {
		cfg.Depth = 1
	}
	return cfg, nil
}
