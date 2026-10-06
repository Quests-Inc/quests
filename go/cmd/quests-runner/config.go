package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type config struct {
	Server string `json:"server"`
	Token  string `json:"token"`
	Label  string `json:"label"`
}

func xdg(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, fallback)
}

func configPath(flagVal string) string {
	if flagVal != "" {
		return flagVal
	}
	return filepath.Join(xdg("XDG_CONFIG_HOME", ".config"), "quests-runner", "config.json")
}

// spoolDir keeps results that could not be delivered yet.
func spoolDir() string {
	return filepath.Join(xdg("XDG_STATE_HOME", ".local/state"), "quests-runner", "spool")
}

func saveConfig(path string, c config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o600)
}

func loadConfig(path string) (config, error) {
	var c config
	b, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return c, err
	}
	// The environment wins over the file (containers, one-off runs).
	if v := os.Getenv("QUESTS_RUNNER_SERVER"); v != "" {
		c.Server = v
	}
	if v := os.Getenv("QUESTS_RUNNER_TOKEN"); v != "" {
		c.Token = v
	}
	if c.Server == "" || c.Token == "" {
		return c, errors.New("config has no server or token")
	}
	return c, nil
}
