package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	BaseURL      string `json:"base_url"`
}

const defaultBaseURL = "https://api.getport.io"

// Load reads config from env vars (PORT_CLIENT_ID, PORT_CLIENT_SECRET, PORT_BASE_URL)
// with fallback to ~/.portcli/config.json.
func Load() (*Config, error) {
	cfg := &Config{
		ClientID:     os.Getenv("PORT_CLIENT_ID"),
		ClientSecret: os.Getenv("PORT_CLIENT_SECRET"),
		BaseURL:      os.Getenv("PORT_BASE_URL"),
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		fileCfg, err := loadFromFile()
		if err == nil {
			if cfg.ClientID == "" {
				cfg.ClientID = fileCfg.ClientID
			}
			if cfg.ClientSecret == "" {
				cfg.ClientSecret = fileCfg.ClientSecret
			}
			if cfg.BaseURL == "" {
				cfg.BaseURL = fileCfg.BaseURL
			}
		}
	}

	if cfg.ClientID == "" || cfg.ClientSecret == "" {
		return nil, fmt.Errorf("missing credentials: set PORT_CLIENT_ID and PORT_CLIENT_SECRET env vars, or create ~/.portcli/config.json")
	}

	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	return cfg, nil
}

func loadFromFile() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(home, ".portcli", "config.json"))
	if err != nil {
		return nil, err
	}
	var cfg Config
	return &cfg, json.Unmarshal(data, &cfg)
}
