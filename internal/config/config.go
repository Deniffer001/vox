package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const appDir = ".vox"

type DashScopeConfig struct {
	APIKey string `json:"api_key,omitempty"`
}

type Services struct {
	DashScope DashScopeConfig `json:"dashscope,omitempty"`
}

type Config struct {
	Services Services `json:"services"`
}

type AppConfig struct {
	Config Config
	Dir    string
}

func Dir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, appDir)
}

func Load() (*AppConfig, error) {
	dir := Dir()
	os.MkdirAll(dir, 0755)
	os.MkdirAll(filepath.Join(dir, "cache"), 0755)

	ac := &AppConfig{Dir: dir}

	configPath := filepath.Join(dir, "config.json")
	if data, err := os.ReadFile(configPath); err == nil {
		// Try new format first
		if err := json.Unmarshal(data, &ac.Config); err != nil || ac.Config.Services.DashScope.APIKey == "" {
			// Try legacy format migration
			var legacy struct {
				Provider string `json:"provider"`
				APIKey   string `json:"api_key"`
			}
			if err := json.Unmarshal(data, &legacy); err == nil && legacy.APIKey != "" {
				ac.Config.Services.DashScope.APIKey = legacy.APIKey
				// Save migrated config
				ac.SaveConfig()
			}
		}
	}

	return ac, nil
}

func (ac *AppConfig) SaveConfig() error {
	return writeJSON(filepath.Join(ac.Dir, "config.json"), ac.Config)
}

func (ac *AppConfig) RequireAPIKey() (string, error) {
	key := ac.Config.Services.DashScope.APIKey
	if key == "" {
		return "", fmt.Errorf("not authenticated — run: vox auth login dashscope --token <key>")
	}
	return key, nil
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
