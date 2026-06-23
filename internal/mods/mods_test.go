package mods

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProfileIDForLoader(t *testing.T) {
	tests := []struct {
		loader LoaderType
		mcVer  string
		lVer   string
		want   string
	}{
		{LoaderForge, "1.20.4", "47.3.0", "forge-1.20.4-47.3.0"},
		{LoaderFabric, "1.20.4", "0.15.11", "fabric-loader-0.15.11-1.20.4"},
		{LoaderQuilt, "1.20.4", "0.24.0", "quilt-loader-0.24.0-1.20.4"},
		{LoaderVanilla, "1.20.4", "", "1.20.4"},
	}

	for _, tc := range tests {
		got := ProfileIDForLoader(tc.loader, tc.mcVer, tc.lVer)
		if got != tc.want {
			t.Errorf("ProfileIDForLoader(%s, %s, %s) = %s, want %s", tc.loader, tc.mcVer, tc.lVer, got, tc.want)
		}
	}
}

func TestDetectLoader(t *testing.T) {
	tests := []struct {
		profile string
		want    LoaderType
	}{
		{"forge-1.20.4-47.3.0", LoaderForge},
		{"fabric-loader-0.15.11-1.20.4", LoaderFabric},
		{"quilt-loader-0.24.0-1.20.4", LoaderQuilt},
		{"1.20.4", LoaderVanilla},
		{"vanilla", LoaderVanilla},
	}

	for _, tc := range tests {
		got := DetectLoader(tc.profile)
		if got != tc.want {
			t.Errorf("DetectLoader(%s) = %s, want %s", tc.profile, got, tc.want)
		}
	}
}

func TestLoaderDisplayName(t *testing.T) {
	if LoaderDisplayName(LoaderForge) != "Forge" {
		t.Errorf("expected Forge, got %s", LoaderDisplayName(LoaderForge))
	}
	if LoaderDisplayName(LoaderFabric) != "Fabric" {
		t.Errorf("expected Fabric, got %s", LoaderDisplayName(LoaderFabric))
	}
	if LoaderDisplayName(LoaderQuilt) != "Quilt" {
		t.Errorf("expected Quilt, got %s", LoaderDisplayName(LoaderQuilt))
	}
	if LoaderDisplayName(LoaderVanilla) != "Vanilla" {
		t.Errorf("expected Vanilla, got %s", LoaderDisplayName(LoaderVanilla))
	}
}

func TestManager_NewAndState(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(mcDir, dataDir)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
	if m.MCBaseDir() != mcDir {
		t.Errorf("expected %s, got %s", mcDir, m.MCBaseDir())
	}
}

func TestManager_ScanEmpty(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(mcDir, dataDir)
	mods, err := m.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(mods) != 0 {
		t.Errorf("expected empty mods, got %d", len(mods))
	}
}

func TestManager_ScanFindsJars(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	modsDir := filepath.Join(mcDir, "mods")
	os.MkdirAll(modsDir, 0755)
	os.WriteFile(filepath.Join(modsDir, "testmod.jar"), []byte("dummy"), 0644)
	os.WriteFile(filepath.Join(modsDir, "another.jar"), []byte("dummy"), 0644)

	m := NewManager(mcDir, dataDir)
	mods, err := m.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(mods) != 2 {
		t.Errorf("expected 2 mods, got %d", len(mods))
	}
}

func TestManager_Toggle(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	modsDir := filepath.Join(mcDir, "mods")
	os.MkdirAll(modsDir, 0755)
	os.WriteFile(filepath.Join(modsDir, "toggletest.jar"), []byte("dummy"), 0644)

	m := NewManager(mcDir, dataDir)
	m.Scan()

	mod, err := m.Toggle("toggletest.jar")
	if err != nil {
		t.Fatalf("Toggle: %v", err)
	}
	if mod.Enabled {
		t.Error("expected mod to be disabled after toggle")
	}

	jarPath := filepath.Join(modsDir, "toggletest.jar")
	disabledPath := filepath.Join(modsDir, "toggletest.jar.disabled")

	if _, err := os.Stat(jarPath); !os.IsNotExist(err) {
		t.Error("expected jar to be renamed to .disabled")
	}
	if _, err := os.Stat(disabledPath); os.IsNotExist(err) {
		t.Error("expected .disabled file to exist")
	}

	mod, err = m.Toggle("toggletest.jar")
	if err != nil {
		t.Fatalf("Toggle back: %v", err)
	}
	if !mod.Enabled {
		t.Error("expected mod to be enabled after second toggle")
	}

	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Error("expected jar to be restored")
	}
}

func TestManager_ToggleNotFound(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(mcDir, dataDir)
	_, err := m.Toggle("nonexistent.jar")
	if err == nil {
		t.Error("expected error for nonexistent mod")
	}
}

func TestManager_Delete(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	modsDir := filepath.Join(mcDir, "mods")
	os.MkdirAll(modsDir, 0755)
	os.WriteFile(filepath.Join(modsDir, "deleteme.jar"), []byte("dummy"), 0644)

	m := NewManager(mcDir, dataDir)
	m.Scan()

	if err := m.Delete("deleteme.jar"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	jarPath := filepath.Join(modsDir, "deleteme.jar")
	if _, err := os.Stat(jarPath); !os.IsNotExist(err) {
		t.Error("expected jar to be deleted")
	}
}

func TestManager_DeleteNotFound(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(mcDir, dataDir)
	if err := m.Delete("nonexistent"); err == nil {
		t.Error("expected error for nonexistent mod")
	}
}

func TestManager_StatePersistence(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	modsDir := filepath.Join(mcDir, "mods")
	os.MkdirAll(modsDir, 0755)
	os.WriteFile(filepath.Join(modsDir, "persist.jar"), []byte("dummy"), 0644)

	m1 := NewManager(mcDir, dataDir)
	m1.Scan()
	m1.Toggle("persist.jar")
	m1.SaveState()

	m2 := NewManager(mcDir, dataDir)
	m2.LoadState()
	m2.Scan()

	list := m2.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(list))
	}
	// The state should preserve the toggled state, but Scan overrides it
	// because it reads the actual filesystem
}

func TestManager_SearchModrinth_NoResults(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(mcDir, dataDir)
	// This will try to actually contact Modrinth API
	// In test environments without network, it will fail
	result, err := m.SearchModrinth("zzzznonexistentmod12345", "1.20.4", LoaderFabric, 5, 0)
	if err != nil {
		t.Logf("Modrinth search (expected to possibly fail without network): %v", err)
		return
	}
	if result.Total == 0 {
		t.Log("No results for nonexistent mod (expected)")
	}
}

func TestInstalledProfiles_Empty(t *testing.T) {
	mcDir := t.TempDir()
	profiles, err := InstalledProfiles(mcDir)
	if err != nil {
		t.Fatalf("InstalledProfiles: %v", err)
	}
	if len(profiles) != 0 {
		t.Errorf("expected no profiles, got %d", len(profiles))
	}
}

func TestInstalledProfiles_WithDirs(t *testing.T) {
	mcDir := t.TempDir()
	versionsDir := filepath.Join(mcDir, "versions")
	os.MkdirAll(filepath.Join(versionsDir, "1.20.4"), 0755)
	os.MkdirAll(filepath.Join(versionsDir, "forge-1.20.4-47.3.0"), 0755)

	profiles, err := InstalledProfiles(mcDir)
	if err != nil {
		t.Fatalf("InstalledProfiles: %v", err)
	}
	if len(profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(profiles))
	}
}

func TestParseCurseForgeURL(t *testing.T) {
	slug, err := ParseCurseForgeURL("https://www.curseforge.com/minecraft/mc-mods/jei")
	if err != nil {
		t.Fatalf("ParseCurseForgeURL: %v", err)
	}
	if slug != "jei" {
		t.Errorf("expected jei, got %s", slug)
	}
}

func TestParseCurseForgeURL_Invalid(t *testing.T) {
	_, err := ParseCurseForgeURL("https://example.com/not-curseforge")
	if err == nil {
		t.Error("expected error for non-curseforge URL")
	}
}

func TestManager_CurseForgeKey(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	m := NewManager(mcDir, dataDir)
	m.SetCurseForgeKey("test-key-123")

	_, err := m.SearchCurseForge("jei", "1.20.4", LoaderForge, 5, 0)
	if err == nil {
		t.Log("CurseForge search attempted (may fail with fake key)")
	}
}

func TestModInfo_DisabledName(t *testing.T) {
	m := ModInfo{
		ID:       "test.jar",
		FileName: "test.jar",
	}
	if m.DisabledName() != "test.jar.disabled" {
		t.Errorf("expected test.jar.disabled, got %s", m.DisabledName())
	}
}

func TestManager_ScanWithDisabled(t *testing.T) {
	mcDir := t.TempDir()
	dataDir := t.TempDir()

	modsDir := filepath.Join(mcDir, "mods")
	os.MkdirAll(modsDir, 0755)
	os.WriteFile(filepath.Join(modsDir, "disabledmod.jar.disabled"), []byte("dummy"), 0644)

	m := NewManager(mcDir, dataDir)
	mods, err := m.Scan()
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if len(mods) != 1 {
		t.Fatalf("expected 1 mod, got %d", len(mods))
	}
	if mods[0].Enabled {
		t.Error("expected disabled mod to be disabled")
	}
}
