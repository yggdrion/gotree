// Package config reads and writes gotree's JSON settings file in the OS config dir.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

const defaultMinutes = 5

// Config is the on-disk settings. Autofetch is nil until the user has been asked.
type Config struct {
	Autofetch *bool `json:"autofetch,omitempty"`
	Minutes   int   `json:"autofetch_interval_minutes,omitempty"`
}

// Path is <user config dir>/gotree/config.json (%AppData%, ~/Library/Application Support, $XDG_CONFIG_HOME or ~/.config).
func Path() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "gotree", "config.json"), nil
}

// Interval is the autofetch period, falling back to the default for unset or invalid values.
func (c Config) Interval() time.Duration {
	if c.Minutes < 1 {
		return defaultMinutes * time.Minute
	}
	return time.Duration(c.Minutes) * time.Minute
}

// Load returns the zero Config when the file is missing or unparseable (both mean: ask again).
func Load() Config {
	var c Config
	p, err := Path()
	if err != nil {
		return c
	}
	b, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(b, &c) != nil {
		return Config{}
	}
	return c
}

// Save writes c, creating the directory if needed.
func Save(c Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if c.Minutes < 1 {
		c.Minutes = defaultMinutes
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}
