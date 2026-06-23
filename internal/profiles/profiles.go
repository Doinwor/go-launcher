package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Profile struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Type          string `json:"type"`
	Created       string `json:"created"`
	LastUsed      string `json:"lastUsed"`
	Icon          string `json:"icon"`
	LastVersionID string `json:"lastVersionId,omitempty"`
	GameDir       string `json:"gameDir,omitempty"`
	JavaDir       string `json:"javaDir,omitempty"`
	JavaArgs      string `json:"javaArgs,omitempty"`
	Resolution    string `json:"resolution,omitempty"`
}

type AuthAccount struct {
	Username    string `json:"username"`
	AccessToken string `json:"accessToken"`
	UUID        string `json:"uuid"`
}

type SelectedUser struct {
	Account string `json:"account"`
	Profile string `json:"profile"`
}

type LauncherVersion struct {
	Name   string `json:"name"`
	Format int    `json:"format"`
}

type LauncherSettings struct {
	EnableSnapshots  bool   `json:"enableSnapshots"`
	EnableHistorical bool   `json:"enableHistorical"`
	KeepLauncherOpen bool   `json:"keepLauncherOpen"`
	ShowGameLog      bool   `json:"showGameLog"`
	Locale           string `json:"locale"`
	ProfileSorting   string `json:"profileSorting"`
	CrashAssistance  bool   `json:"crashAssistance"`
	EnableAdvanced   bool   `json:"enableAdvanced"`
	SilentErrors     bool   `json:"silentErrors"`
	EnableAnalytics  bool   `json:"enableAnalytics"`
}

type LauncherProfiles struct {
	Profiles                map[string]Profile            `json:"profiles"`
	SelectedProfile         string                        `json:"selectedProfile"`
	ClientToken             string                        `json:"clientToken"`
	AuthenticationDatabase  map[string]AuthAccount         `json:"authenticationDatabase"`
	SelectedUser            SelectedUser                  `json:"selectedUser"`
	LauncherVersion         LauncherVersion               `json:"launcherVersion"`
	Settings                LauncherSettings              `json:"settings"`
}

type Store struct {
	mu       sync.RWMutex
	data     *LauncherProfiles
	filename string
}

func NewStore() *Store {
	return &Store{
		data:     DefaultStore().data,
		filename: defaultProfilesPath(),
	}
}

func DefaultStore() *Store {
	return &Store{
		data: &LauncherProfiles{
			Profiles:               make(map[string]Profile),
			SelectedProfile:        "",
			ClientToken:            newClientToken(),
			AuthenticationDatabase: make(map[string]AuthAccount),
			SelectedUser:           SelectedUser{},
			LauncherVersion:        LauncherVersion{Name: "1.0.0", Format: 1},
			Settings:               defaultSettings(),
		},
		filename: defaultProfilesPath(),
	}
}

func defaultSettings() LauncherSettings {
	return LauncherSettings{
		EnableSnapshots:  false,
		EnableHistorical: false,
		KeepLauncherOpen: false,
		ShowGameLog:      true,
		Locale:           "ru-ru",
		ProfileSorting:   "byLastPlayed",
		EnableAdvanced:   false,
		SilentErrors:     false,
		EnableAnalytics:  false,
	}
}

func defaultProfilesPath() string {
	mcDir := filepath.Join(os.Getenv("APPDATA"), ".minecraft")
	return filepath.Join(mcDir, "launcher_profiles.json")
}

func newClientToken() string {
	b := make([]byte, 16)
	for i := range b {
		b[i] = byte(i*7 + 13)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func (s *Store) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.filename)
	if err != nil {
		return err
	}

	var profiles LauncherProfiles
	if err := json.Unmarshal(data, &profiles); err != nil {
		return err
	}
	if profiles.Profiles == nil {
		profiles.Profiles = make(map[string]Profile)
	}
	if profiles.AuthenticationDatabase == nil {
		profiles.AuthenticationDatabase = make(map[string]AuthAccount)
	}
	s.data = &profiles
	return nil
}

func (s *Store) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := filepath.Dir(s.filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(s.filename, data, 0644)
}

func (s *Store) UpsertProfile(p Profile) {
	if s.data.Profiles == nil {
		s.data.Profiles = make(map[string]Profile)
	}
	s.data.Profiles[p.ID] = p
}

func (s *Store) SetSelectedProfile(id string) {
	s.data.SelectedProfile = id
}

func (s *Store) SetAuthAccount(uuid, username, token string) {
	if s.data.AuthenticationDatabase == nil {
		s.data.AuthenticationDatabase = make(map[string]AuthAccount)
	}
	s.data.AuthenticationDatabase[uuid] = AuthAccount{
		Username:    username,
		AccessToken: token,
		UUID:        uuid,
	}
}

func (s *Store) DeleteProfile(id string) {
	delete(s.data.Profiles, id)
	delete(s.data.AuthenticationDatabase, id)
}

func (s *Store) GetProfiles() map[string]Profile {
	return s.data.Profiles
}

func (s *Store) GetProfile(id string) (Profile, bool) {
	p, ok := s.data.Profiles[id]
	return p, ok
}

func (s *Store) GetData() *LauncherProfiles {
	return s.data
}

func (s *Store) SetFilename(fn string) {
	s.filename = fn
}
