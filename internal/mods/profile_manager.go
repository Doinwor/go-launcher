package mods

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type ModProfile struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Created   string   `json:"created"`
	ModIDs    []string `json:"modIds"`
}

type modProfilesData struct {
	Profiles      map[string]*ModProfile `json:"profiles"`
	ActiveProfile string                 `json:"activeProfile"`
}

type ProfileManager struct {
	mu        sync.RWMutex
	filePath  string
	data      *modProfilesData
	modsMan   *Manager
}

func NewProfileManager(appDataDir string, modsMan *Manager) *ProfileManager {
	pm := &ProfileManager{
		filePath: filepath.Join(appDataDir, "mod_profiles.json"),
		data: &modProfilesData{
			Profiles: make(map[string]*ModProfile),
		},
		modsMan: modsMan,
	}
	pm.load()
	return pm
}

func (pm *ProfileManager) load() {
	data, err := os.ReadFile(pm.filePath)
	if err != nil {
		return
	}
	var d modProfilesData
	if err := json.Unmarshal(data, &d); err != nil {
		return
	}
	if d.Profiles == nil {
		d.Profiles = make(map[string]*ModProfile)
	}
	pm.data = &d
}

func (pm *ProfileManager) save() error {
	dir := filepath.Dir(pm.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pm.data, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(pm.filePath, data, 0644)
}

func (pm *ProfileManager) List() []ModProfile {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]ModProfile, 0, len(pm.data.Profiles))
	for _, p := range pm.data.Profiles {
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result
}

func (pm *ProfileManager) Get(id string) (*ModProfile, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.data.Profiles[id]
	if !ok {
		return nil, false
	}
	cp := *p
	return &cp, true
}

func (pm *ProfileManager) Create(name string) (*ModProfile, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	id := name
	if _, exists := pm.data.Profiles[id]; exists {
		return nil, os.ErrExist
	}

	p := &ModProfile{
		ID:      id,
		Name:    name,
		Created: time.Now().UTC().Format(time.RFC3339),
		ModIDs:  []string{},
	}
	pm.data.Profiles[id] = p
	if err := pm.save(); err != nil {
		delete(pm.data.Profiles, id)
		return nil, err
	}
	cp := *p
	return &cp, nil
}

func (pm *ProfileManager) Delete(id string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, ok := pm.data.Profiles[id]; !ok {
		return os.ErrNotExist
	}

	if pm.data.ActiveProfile == id {
		pm.data.ActiveProfile = ""
	}

	delete(pm.data.Profiles, id)
	return pm.save()
}

func (pm *ProfileManager) ActiveID() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return pm.data.ActiveProfile
}

func (pm *ProfileManager) EnsureVersionProfile(versionID string) *ModProfile {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if p, ok := pm.data.Profiles[versionID]; ok {
		return p
	}

	p := &ModProfile{
		ID:      versionID,
		Name:    versionID,
		Created: time.Now().UTC().Format(time.RFC3339),
		ModIDs:  []string{},
	}
	pm.data.Profiles[versionID] = p
	pm.save()
	return p
}

func (pm *ProfileManager) Activate(id string) error {
	pm.mu.Lock()

	if id != "" {
		if _, ok := pm.data.Profiles[id]; !ok {
			pm.mu.Unlock()
			return os.ErrNotExist
		}
	}

	oldActive := pm.data.ActiveProfile
	pm.data.ActiveProfile = id
	pm.mu.Unlock()

	// Disable all mods, then enable only those in the target profile
	var targetMods []string
	if id != "" {
		pm.mu.RLock()
		targetMods = pm.data.Profiles[id].ModIDs
		pm.mu.RUnlock()
	}

	return pm.applyProfileMods(oldActive, id, targetMods)
}

func (pm *ProfileManager) Deactivate() error {
	return pm.Activate("")
}

func (pm *ProfileManager) applyProfileMods(oldActive, newActive string, enableIDs []string) error {
	enableSet := make(map[string]bool)
	for _, id := range enableIDs {
		enableSet[id] = true
	}

	allMods := pm.modsMan.List()

	var errs []error
	for _, m := range allMods {
		shouldEnable := enableSet[m.FileName]
		if m.Enabled && !shouldEnable {
			if _, err := pm.modsMan.Toggle(m.FileName); err != nil {
				errs = append(errs, err)
			}
		} else if !m.Enabled && shouldEnable {
			if _, err := pm.modsMan.Toggle(m.FileName); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

func (pm *ProfileManager) AddMod(profileID, modID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, ok := pm.data.Profiles[profileID]
	if !ok {
		return os.ErrNotExist
	}

	for _, id := range p.ModIDs {
		if id == modID {
			return nil
		}
	}

	p.ModIDs = append(p.ModIDs, modID)
	return pm.save()
}

func (pm *ProfileManager) RemoveMod(profileID, modID string) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	p, ok := pm.data.Profiles[profileID]
	if !ok {
		return os.ErrNotExist
	}

	filtered := make([]string, 0, len(p.ModIDs))
	for _, id := range p.ModIDs {
		if id != modID {
			filtered = append(filtered, id)
		}
	}
	p.ModIDs = filtered
	return pm.save()
}

func (pm *ProfileManager) ToggleModInActive(modID string) (bool, error) {
	pm.mu.Lock()
	activeID := pm.data.ActiveProfile
	if activeID == "" {
		pm.mu.Unlock()
		return false, nil
	}

	p, ok := pm.data.Profiles[activeID]
	if !ok {
		pm.mu.Unlock()
		return false, nil
	}

	found := false
	idx := -1
	for i, id := range p.ModIDs {
		if id == modID {
			found = true
			idx = i
			break
		}
	}

	if found {
		p.ModIDs = append(p.ModIDs[:idx], p.ModIDs[idx+1:]...)
	} else {
		p.ModIDs = append(p.ModIDs, modID)
	}
	pm.mu.Unlock()

	if err := pm.save(); err != nil {
		return false, err
	}

	return !found, nil
}

func (pm *ProfileManager) IsModInProfile(profileID, modID string) bool {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.data.Profiles[profileID]
	if !ok {
		return false
	}
	for _, id := range p.ModIDs {
		if id == modID {
			return true
		}
	}
	return false
}

func (pm *ProfileManager) IsModInActiveProfile(modID string) bool {
	pm.mu.RLock()
	activeID := pm.data.ActiveProfile
	if activeID == "" {
		pm.mu.RUnlock()
		return false
	}
	pm.mu.RUnlock()
	return pm.IsModInProfile(activeID, modID)
}

func (pm *ProfileManager) ModIDsInProfile(profileID string) []string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.data.Profiles[profileID]
	if !ok {
		return nil
	}
	result := make([]string, len(p.ModIDs))
	copy(result, p.ModIDs)
	return result
}

func (pm *ProfileManager) AddModToActive(modID string) {
	pm.mu.Lock()
	activeID := pm.data.ActiveProfile
	if activeID == "" {
		pm.mu.Unlock()
		return
	}
	p, ok := pm.data.Profiles[activeID]
	if !ok {
		pm.mu.Unlock()
		return
	}
	for _, id := range p.ModIDs {
		if id == modID {
			pm.mu.Unlock()
			return
		}
	}
	p.ModIDs = append(p.ModIDs, modID)
	pm.mu.Unlock()
	pm.save()
}
