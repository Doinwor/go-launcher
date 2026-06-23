package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
)

type FabricMeta struct {
	Loader struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	} `json:"loader"`
	Intermediary struct {
		Version string `json:"version"`
	} `json:"intermediary"`
	LauncherMeta struct {
		Version int `json:"version"`
	} `json:"launcherMeta"`
}

type FabricMetaResponse []FabricMeta

type QuiltMeta struct {
	Loader struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
	} `json:"loader"`
}

type QuiltMetaResponse []QuiltMeta

type ForgeMeta struct {
	Webpath    string `json:"webpath"`
	Version    string `json:"version"`
	MCVersion  string `json:"mcversion"`
	Build      int    `json:"build"`
	Branch     string `json:"branch"`
}

type ForgePromos struct {
	Promos map[string]string `json:"promos"`
}

func FetchFabricVersions(mcVersion string) ([]FabricMeta, error) {
	url := fmt.Sprintf("https://meta.fabricmc.net/v2/versions/loader/%s", mcVersion)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fabric meta request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fabric meta HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("fabric meta read: %w", err)
	}

	var meta FabricMetaResponse
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("fabric meta parse: %w", err)
	}

	return meta, nil
}

func FetchFabricLatestLoader(mcVersion string) (string, error) {
	versions, err := FetchFabricVersions(mcVersion)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", ErrLoaderNotFound
	}

	for _, v := range versions {
		if v.Loader.Stable {
			return v.Loader.Version, nil
		}
	}

	return versions[0].Loader.Version, nil
}

func FetchQuiltVersions(mcVersion string) ([]QuiltMeta, error) {
	url := fmt.Sprintf("https://meta.quiltmc.org/v3/versions/loader/%s", mcVersion)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("quilt meta request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("quilt meta HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("quilt meta read: %w", err)
	}

	var meta QuiltMetaResponse
	if err := json.Unmarshal(body, &meta); err != nil {
		return nil, fmt.Errorf("quilt meta parse: %w", err)
	}

	return meta, nil
}

func FetchQuiltLatestLoader(mcVersion string) (string, error) {
	versions, err := FetchQuiltVersions(mcVersion)
	if err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", ErrLoaderNotFound
	}

	for _, v := range versions {
		if v.Loader.Stable {
			return v.Loader.Version, nil
		}
	}

	return versions[0].Loader.Version, nil
}

func FetchForgeVersions(mcVersion string) ([]ForgeMeta, error) {
	url := fmt.Sprintf("https://files.minecraftforge.net/net/minecraftforge/forge/maven-metadata.json")
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("forge meta request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("forge meta HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("forge meta read: %w", err)
	}

	var allVersions map[string]ForgeMeta
	if err := json.Unmarshal(body, &allVersions); err != nil {
		var promos ForgePromos
		if err2 := json.Unmarshal(body, &promos); err2 == nil {
			return parseForgePromos(promos, mcVersion), nil
		}
		return nil, fmt.Errorf("forge meta parse: %w", err)
	}

	var matched []ForgeMeta
	for key, ver := range allVersions {
		if ver.MCVersion == mcVersion || strings.HasPrefix(key, mcVersion+"-") {
			ver.Version = key
			matched = append(matched, ver)
		}
	}
	if len(matched) == 0 {
		return nil, ErrLoaderNotFound
	}

	return matched, nil
}

func parseForgePromos(promos ForgePromos, mcVersion string) []ForgeMeta {
	var metas []ForgeMeta
	for key, ver := range promos.Promos {
		if strings.HasPrefix(key, mcVersion+"-") || key == mcVersion+"-recommended" || key == mcVersion+"-latest" {
			metas = append(metas, ForgeMeta{
				Version:   ver,
				MCVersion: mcVersion,
			})
		}
	}
	if len(metas) == 0 {
		if ver, ok := promos.Promos[mcVersion+"-recommended"]; ok {
			metas = append(metas, ForgeMeta{Version: ver, MCVersion: mcVersion})
		} else if ver, ok := promos.Promos[mcVersion+"-latest"]; ok {
			metas = append(metas, ForgeMeta{Version: ver, MCVersion: mcVersion})
		}
	}
	return metas
}

func FetchForgeLatestLoader(mcVersion string) (string, error) {
	versions, err := FetchForgeVersions(mcVersion)
	if err != nil {
		return "", err
	}
	return versions[0].Version, nil
}

func InstallLoader(req InstallLoaderReq, mcDir, javaPath string) (*InstallLoaderResp, error) {
	loaderVersion := req.LoaderVersion

	switch req.Loader {
	case LoaderFabric:
		if loaderVersion == "" {
			var err error
			loaderVersion, err = FetchFabricLatestLoader(req.MCVersion)
			if err != nil {
				return nil, fmt.Errorf("fabric version lookup: %w", err)
			}
		}
		return installFabric(req.MCVersion, loaderVersion, mcDir, javaPath)

	case LoaderQuilt:
		if loaderVersion == "" {
			var err error
			loaderVersion, err = FetchQuiltLatestLoader(req.MCVersion)
			if err != nil {
				return nil, fmt.Errorf("quilt version lookup: %w", err)
			}
		}
		return installQuilt(req.MCVersion, loaderVersion, mcDir, javaPath)

	case LoaderForge:
		if loaderVersion == "" {
			var err error
			loaderVersion, err = FetchForgeLatestLoader(req.MCVersion)
			if err != nil {
				return nil, fmt.Errorf("forge version lookup: %w", err)
			}
		}
		return installForge(req.MCVersion, loaderVersion, mcDir, javaPath)

	default:
		return nil, fmt.Errorf("unsupported loader: %s", req.Loader)
	}
}

func installerPath(mcDir, filename string) string {
	return filepath.Join(mcDir, ".launcher", filename)
}

func installFabric(mcVersion, loaderVersion, mcDir, javaPath string) (*InstallLoaderResp, error) {
	installerVer := "1.0.1"
	jarName := fmt.Sprintf("fabric-installer-%s.jar", installerVer)
	jarPath := installerPath(mcDir, jarName)
	installerURL := fmt.Sprintf("https://maven.fabricmc.net/net/fabricmc/fabric-installer/%s/%s", installerVer, jarName)

	if err := downloadJar(jarPath, installerURL); err != nil {
		return nil, fmt.Errorf("download fabric installer: %w", err)
	}

	profileID := ProfileIDForLoader(LoaderFabric, mcVersion, loaderVersion)

	args := []string{
		"-jar", jarPath,
		"client",
		"-dir", mcDir,
		"-mcversion", mcVersion,
		"-loader", loaderVersion,
		"-noprofile",
	}

	cmd := exec.Command(javaPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("fabric installer failed: %w", err)
	}

	return &InstallLoaderResp{
		Loader:        LoaderFabric,
		MCVersion:     mcVersion,
		LoaderVersion: loaderVersion,
		ProfileID:     profileID,
		JavaPath:      javaPath,
	}, nil
}

func installQuilt(mcVersion, loaderVersion, mcDir, javaPath string) (*InstallLoaderResp, error) {
	installerVer := "0.8.0"
	jarName := fmt.Sprintf("quilt-installer-%s.jar", installerVer)
	jarPath := installerPath(mcDir, jarName)
	installerURL := fmt.Sprintf("https://maven.quiltmc.org/repository/release/org/quiltmc/quilt-installer/%[1]s/%[2]s", installerVer, jarName)

	if err := downloadJar(jarPath, installerURL); err != nil {
		return nil, fmt.Errorf("download quilt installer: %w", err)
	}

	profileID := ProfileIDForLoader(LoaderQuilt, mcVersion, loaderVersion)

	args := []string{
		"-jar", jarPath,
		"install",
		"client",
		mcVersion,
		"--loader-version", loaderVersion,
		"--install-dir", mcDir,
		"--no-profile",
	}

	cmd := exec.Command(javaPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("quilt installer failed: %w", err)
	}

	return &InstallLoaderResp{
		Loader:        LoaderQuilt,
		MCVersion:     mcVersion,
		LoaderVersion: loaderVersion,
		ProfileID:     profileID,
		JavaPath:      javaPath,
	}, nil
}

func installForge(mcVersion, forgeVersion, mcDir, javaPath string) (*InstallLoaderResp, error) {
	jarName := fmt.Sprintf("forge-%s-%s-installer.jar", mcVersion, forgeVersion)
	jarPath := installerPath(mcDir, jarName)
	installerURL := fmt.Sprintf("https://files.minecraftforge.net/maven/net/minecraftforge/forge/%[1]s-%[2]s/%[3]s", mcVersion, forgeVersion, jarName)

	if err := downloadJar(jarPath, installerURL); err != nil {
		return nil, fmt.Errorf("download forge installer: %w", err)
	}

	profileID := ProfileIDForLoader(LoaderForge, mcVersion, forgeVersion)

	args := []string{
		"-jar", jarPath,
		"--installClient",
		mcDir,
	}

	cmd := exec.Command(javaPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if runtime.GOOS == "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	}

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("forge installer failed: %w", err)
	}

	return &InstallLoaderResp{
		Loader:        LoaderForge,
		MCVersion:     mcVersion,
		LoaderVersion: forgeVersion,
		ProfileID:     profileID,
		JavaPath:      javaPath,
	}, nil
}

func downloadJar(destPath, url string) error {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	if _, err := os.Stat(destPath); err == nil {
		return nil
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

func InstalledProfiles(mcDir string) ([]string, error) {
	versionsDir := filepath.Join(mcDir, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var profiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			profiles = append(profiles, entry.Name())
		}
	}
	return profiles, nil
}

func DetectLoader(profileID string) LoaderType {
	if strings.HasPrefix(profileID, "forge-") {
		return LoaderForge
	}
	if strings.HasPrefix(profileID, "fabric-loader-") || strings.HasPrefix(profileID, "fabric-") {
		return LoaderFabric
	}
	if strings.HasPrefix(profileID, "quilt-loader-") || strings.HasPrefix(profileID, "quilt-") {
		return LoaderQuilt
	}
	return LoaderVanilla
}
