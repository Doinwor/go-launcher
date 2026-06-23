package configs

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type AppConfig struct {
	Port          string `json:"port"`
	StaticDir     string `json:"staticDir"`
	ProfilesFile  string `json:"profilesFile"`
	SettingsFile  string `json:"settingsFile"`
	MinecraftDir  string `json:"minecraftDir"`
	JavaPath      string `json:"javaPath"`
	MinMemory     int    `json:"minMemory"`
	MaxMemory     int    `json:"maxMemory"`
	Workers       int    `json:"workers"`
}

func DefaultConfig() *AppConfig {
	appData := os.Getenv("APPDATA")
	return &AppConfig{
		Port:         "8080",
		StaticDir:    "web",
		ProfilesFile: filepath.Join(appData, "offline-launcher", "launcher_profiles.json"),
		SettingsFile: filepath.Join(appData, "offline-launcher", "settings.json"),
		MinecraftDir: filepath.Join(appData, ".minecraft"),
		MinMemory:    1024,
		MaxMemory:    4096,
		Workers:      4,
	}
}

func LoadConfig(path string) (*AppConfig, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, nil
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *AppConfig) Save(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
