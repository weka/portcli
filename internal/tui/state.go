package tui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// defaultView is where the TUI opens when there is nothing remembered, and
// the fallback whenever a remembered view no longer resolves.
const defaultView = "blueprints"

// State is the small amount of session state worth surviving a restart.
type State struct {
	LastView    string `json:"last_view"`
	Refresh     string `json:"refresh,omitempty"`
	WideColumns bool   `json:"wide_columns,omitempty"`
}

// statePath is ~/.portcli/tui.json, beside the credentials file the CLI
// already uses.
func statePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".portcli", "tui.json"), nil
}

// LoadState reads remembered state. Every failure is silent and yields
// defaults: a stale or corrupt state file must never be able to stop the TUI
// from opening.
func LoadState() State {
	st := State{LastView: defaultView}
	path, err := statePath()
	if err != nil {
		return st
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return st
	}
	var loaded State
	if err := json.Unmarshal(data, &loaded); err != nil {
		return st
	}
	if loaded.LastView == "" {
		loaded.LastView = defaultView
	}
	return loaded
}

// Save writes state, creating ~/.portcli if needed. Errors are returned for
// the caller to flash; nothing about a failed save should interrupt the user.
func (s State) Save() error {
	path, err := statePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// RefreshInterval returns the configured interval, or fallback when unset or
// unparsable.
func (s State) RefreshInterval(fallback time.Duration) time.Duration {
	if s.Refresh == "" {
		return fallback
	}
	d, err := time.ParseDuration(s.Refresh)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
