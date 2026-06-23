package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type AppSettings struct {
	Theme              string `json:"theme"`
	Language           string `json:"language"`
	MinecraftDir       string `json:"minecraftDir"`
	JavaPath           string `json:"javaPath"`
	MinMemory          int    `json:"minMemory"`
	MaxMemory          int    `json:"maxMemory"`
	WindowWidth        int    `json:"windowWidth"`
	WindowHeight       int    `json:"windowHeight"`
	AutoConnect        bool   `json:"autoConnect"`
	CheckUpdates       bool   `json:"checkUpdates"`
	UseAuthlibInjector bool   `json:"useAuthlibInjector"`
	AuthlibInjectorURL string `json:"authlibInjectorURL"`
	AuthServerEnabled  bool   `json:"authServerEnabled"`
	AuthServerPort     int    `json:"authServerPort"`
	CurseForgeKey      string `json:"curseForgeKey,omitempty"`
}

type Manager struct {
	mu       sync.RWMutex
	settings *AppSettings
	filename string
}

func NewManager() *Manager {
	return &Manager{
		settings: DefaultSettings(),
		filename: defaultPath(),
	}
}

func DefaultSettings() *AppSettings {
	return &AppSettings{
		Theme:              "dark",
		Language:           "ru-ru",
		MinecraftDir:       filepath.Join(os.Getenv("APPDATA"), ".minecraft"),
		JavaPath:           findJava(),
		MinMemory:          1024,
		MaxMemory:          4096,
		WindowWidth:        854,
		WindowHeight:       480,
		AutoConnect:        true,
		CheckUpdates:       true,
		UseAuthlibInjector: false,
		AuthlibInjectorURL: "http://localhost:25566",
		AuthServerEnabled:  false,
		AuthServerPort:     25566,
	}
}

func findJava() string {
	candidates := []string{
		filepath.Join(os.Getenv("ProgramFiles"), "Java", "bin", "java.exe"),
		filepath.Join(os.Getenv("ProgramFiles(x86)"), "Java", "bin", "java.exe"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "java"
}

func defaultPath() string {
	dir := filepath.Join(os.Getenv("APPDATA"), "offline-launcher")
	return filepath.Join(dir, "settings.json")
}

func (m *Manager) Load() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filename)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, m.settings)
}

func (m *Manager) Save() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	dir := filepath.Dir(m.filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(m.settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(m.filename, data, 0644)
}

func (m *Manager) Get() *AppSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

func (m *Manager) Update(s *AppSettings) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = s
	return nil
}

func (m *Manager) GetSettingsPath() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filename
}
