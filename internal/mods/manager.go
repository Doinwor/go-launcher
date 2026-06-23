package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Manager struct {
	mu            sync.RWMutex
	mcDir         string
	stateFile     string
	mods          map[string]*ModInfo
	cfClient      *CurseForgeClient
}

func NewManager(mcDir, appDataDir string) *Manager {
	return &Manager{
		mcDir:     mcDir,
		stateFile: filepath.Join(appDataDir, "mods_state.json"),
		mods:      make(map[string]*ModInfo),
	}
}

func (m *Manager) SetCurseForgeKey(apiKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if apiKey != "" {
		m.cfClient = NewCurseForgeClient(apiKey)
	} else {
		m.cfClient = nil
	}
}

func (m *Manager) LoadState() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var mods []ModInfo
	if err := json.Unmarshal(data, &mods); err != nil {
		return err
	}

	m.mods = make(map[string]*ModInfo)
	for i := range mods {
		m.mods[mods[i].ID] = &mods[i]
	}

	return nil
}

func (m *Manager) SaveState() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mods := make([]ModInfo, 0, len(m.mods))
	for _, mod := range m.mods {
		mods = append(mods, *mod)
	}

	sort.Slice(mods, func(i, j int) bool {
		return mods[i].Name < mods[j].Name
	})

	data, err := json.MarshalIndent(mods, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(m.stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(m.stateFile, data, 0644)
}

func (m *Manager) Scan() ([]ModInfo, error) {
	modsDir := filepath.Join(m.mcDir, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(modsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	seen := make(map[string]bool)

	for _, entry := range entries {
		name := entry.Name()

		enabled := true
		modID := name
		if strings.HasSuffix(name, ".disabled") {
			enabled = false
			modID = strings.TrimSuffix(name, ".disabled")
		}

		if filepath.Ext(modID) != ".jar" {
			continue
		}

		seen[modID] = true

		if existing, ok := m.mods[modID]; ok {
			existing.Enabled = enabled
		} else {
			info, _ := entry.Info()
			m.mods[modID] = &ModInfo{
				ID:       modID,
				Name:     strings.TrimSuffix(modID, ".jar"),
				FileName: modID,
				FileSize: info.Size(),
				Enabled:  enabled,
				Source:   SourceDirect,
			}
		}
	}

	for id := range m.mods {
		if !seen[id] && !strings.HasSuffix(id, ".disabled") {
			if _, err := os.Stat(filepath.Join(modsDir, id)); os.IsNotExist(err) {
				delete(m.mods, id)
			}
		}
	}

	result := make([]ModInfo, 0, len(m.mods))
	for _, mod := range m.mods {
		result = append(result, *mod)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result, nil
}

func (m *Manager) List() []ModInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]ModInfo, 0, len(m.mods))
	for _, mod := range m.mods {
		result = append(result, *mod)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

func (m *Manager) Install(projectID, versionID, downloadURL, gameVersion, profileID string, loader LoaderType, source ModSource) (*ModInfo, error) {
	modsDir := filepath.Join(m.mcDir, "mods")
	if err := os.MkdirAll(modsDir, 0755); err != nil {
		return nil, err
	}

	modInfo := &ModInfo{
		ProjectID:   projectID,
		GameVersion: gameVersion,
		Loader:      loader,
		Source:      source,
		Enabled:     true,
	}

	switch source {
	case SourceModrinth:
		mv, err := FetchModrinthVersion(projectID, gameVersion, loader)
		if err != nil {
			return nil, fmt.Errorf("modrinth version fetch: %w", err)
		}
		if len(mv.Files) == 0 {
			return nil, fmt.Errorf("no files in modrinth version %s", mv.ID)
		}
		file := mv.Files[0]
		destPath := filepath.Join(modsDir, file.Filename)
		if err := downloadMod(destPath, file.URL); err != nil {
			return nil, err
		}
		modInfo.ID = removeJar(file.Filename)
		modInfo.Name = mv.Name
		modInfo.Version = mv.VersionNum
		modInfo.FileName = file.Filename
		modInfo.FileSize = file.Size
		modInfo.URL = file.URL

	case SourceCurseForge:
		if m.cfClient == nil {
			return nil, fmt.Errorf("CurseForge API key not configured")
		}
		dlURL, err := m.cfClient.ModURL(projectID)
		if err != nil {
			return nil, fmt.Errorf("curseforge mod URL: %w", err)
		}
		if downloadURL != "" {
			dlURL = downloadURL
		}
		fileName := projectID + ".jar"
		if urlParts := strings.Split(dlURL, "/"); len(urlParts) > 0 {
			if last := urlParts[len(urlParts)-1]; strings.HasSuffix(last, ".jar") {
				fileName = last
			}
		}
		destPath := filepath.Join(modsDir, fileName)
		if err := downloadMod(destPath, dlURL); err != nil {
			return nil, err
		}
		modInfo.ID = removeJar(fileName)
		modInfo.Name = removeJar(fileName)
		modInfo.FileName = fileName
		modInfo.Version = versionID
		modInfo.URL = dlURL

	case SourceDirect:
		if downloadURL == "" {
			return nil, fmt.Errorf("download URL required for direct mod install")
		}
		fileName := projectID + ".jar"
		if urlParts := strings.Split(downloadURL, "/"); len(urlParts) > 0 {
			if last := urlParts[len(urlParts)-1]; strings.HasSuffix(last, ".jar") {
				fileName = last
			}
		}
		destPath := filepath.Join(modsDir, fileName)
		if err := downloadMod(destPath, downloadURL); err != nil {
			return nil, err
		}
		modInfo.ID = removeJar(fileName)
		modInfo.Name = removeJar(fileName)
		modInfo.FileName = fileName
		modInfo.URL = downloadURL

	default:
		return nil, fmt.Errorf("unsupported mod source: %s", source)
	}

	m.mu.Lock()
	m.mods[modInfo.FileName] = modInfo
	m.mu.Unlock()

	m.SaveState()

	return modInfo, nil
}

func (m *Manager) Toggle(id string) (*ModInfo, error) {
	modsDir := filepath.Join(m.mcDir, "mods")

	m.mu.Lock()

	mod, ok := m.mods[id]
	if !ok {
		m.mu.Unlock()
		return nil, fmt.Errorf("mod not found: %s", id)
	}

	jarPath := filepath.Join(modsDir, mod.FileName)
	disabledPath := filepath.Join(modsDir, mod.FileName+".disabled")

	if mod.Enabled {
		if _, err := os.Stat(jarPath); err == nil {
			if err := os.Rename(jarPath, disabledPath); err != nil {
				m.mu.Unlock()
				return nil, fmt.Errorf("disable rename: %w", err)
			}
		}
		mod.Enabled = false
	} else {
		if _, err := os.Stat(disabledPath); err == nil {
			if err := os.Rename(disabledPath, jarPath); err != nil {
				m.mu.Unlock()
				return nil, fmt.Errorf("enable rename: %w", err)
			}
		}
		mod.Enabled = true
	}

	m.mu.Unlock()

	m.SaveState()

	return mod, nil
}

func (m *Manager) Delete(id string) error {
	modsDir := filepath.Join(m.mcDir, "mods")

	m.mu.Lock()

	mod, ok := m.mods[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("mod not found: %s", id)
	}

	jarPath := filepath.Join(modsDir, mod.FileName)
	disabledPath := filepath.Join(modsDir, mod.FileName+".disabled")

	os.Remove(jarPath)
	os.Remove(disabledPath)

	delete(m.mods, id)
	m.mu.Unlock()

	m.SaveState()

	return nil
}

func (m *Manager) SearchModrinth(query, gameVersion string, loader LoaderType, limit, offset int) (*SearchResult, error) {
	return SearchModrinth(query, gameVersion, loader, limit, offset)
}

func (m *Manager) SearchCurseForge(query, gameVersion string, loader LoaderType, limit, offset int) (*SearchResult, error) {
	if m.cfClient == nil {
		return nil, fmt.Errorf("CurseForge API key not configured")
	}
	return m.cfClient.Search(query, gameVersion, loader, limit, offset)
}

func (m *Manager) MCBaseDir() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.mcDir
}

func (m *Manager) SetMinecraftDir(mcDir string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mcDir = mcDir
}

func downloadMod(destPath, url string) error {
	if _, err := os.Stat(destPath); err == nil {
		return nil
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	out, err := os.Create(destPath + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		os.Remove(destPath + ".tmp")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		os.Remove(destPath + ".tmp")
		return fmt.Errorf("HTTP %d downloading %s", resp.StatusCode, url)
	}

	if _, err := io.Copy(out, resp.Body); err != nil {
		os.Remove(destPath + ".tmp")
		return err
	}

	if err := os.Rename(destPath+".tmp", destPath); err != nil {
		return err
	}

	return nil
}

func removeJar(name string) string {
	return strings.TrimSuffix(name, ".jar")
}
