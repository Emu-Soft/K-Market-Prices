package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

const defaultFeed = "https://raw.githubusercontent.com/Emu-Soft/K-Market-Prices/data/"

// Config lives in %APPDATA%\K-Market Prices\config.json. AddOnsDir can be set
// by hand when the folder is not found on its own.
type Config struct {
	AddOnsDir string `json:"addonsDir"`
	FeedURL   string `json:"feedUrl"`
	Minutes   int    `json:"checkEveryMinutes"`
}

func configPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "K-Market Prices", "config.json")
}

func LoadConfig(path string) Config {
	c := Config{}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &c)
	}
	if c.FeedURL == "" {
		c.FeedURL = defaultFeed
	}
	if c.Minutes < 5 {
		c.Minutes = 10
	}
	return c
}

func SaveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return writeAtomic(path, b)
}
