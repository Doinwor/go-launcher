package download

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/offline-launcher/internal/download/types"
	"github.com/offline-launcher/internal/launch"
	"github.com/offline-launcher/internal/log"
)

type Manager struct {
	MinecraftDir string
	Workers      int
	Progress     chan types.ProgressInfo
	client       *httpClient
}

func NewManager(minecraftDir string, workers int) *Manager {
	if workers < 1 {
		workers = 4
	}
	return &Manager{
		MinecraftDir: minecraftDir,
		Workers:      workers,
		Progress:     make(chan types.ProgressInfo, 64),
		client:       defaultClient(),
	}
}

func (m *Manager) FetchManifest() (*types.VersionManifest, error) {
	return fetchManifest(m.client)
}

func (m *Manager) FetchVersion(id string) (*types.VersionInfo, error) {
	manifest, err := m.FetchManifest()
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == id {
			versionURL = v.URL
			break
		}
	}
	if versionURL == "" {
		return nil, fmt.Errorf("version %q not found in manifest", id)
	}

	vi, err := fetchVersion(m.client, versionURL)
	if err != nil {
		return nil, err
	}
	vi.URL = versionURL
	return vi, nil
}

func (m *Manager) IsVersionInstalled(id string) bool {
	versionDir := filepath.Join(m.MinecraftDir, "versions", id)
	jarFile := filepath.Join(versionDir, id+".jar")
	jsonFile := filepath.Join(versionDir, id+".json")

	ji, errJ := os.Stat(jarFile)
	_, errJSON := os.Stat(jsonFile)
	if errJ != nil || errJSON != nil {
		return false
	}
	if ji.Size() == 0 {
		return false
	}
	return true
}

func (m *Manager) InstalledVersions() ([]string, error) {
	versionsDir := filepath.Join(m.MinecraftDir, "versions")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			jar := filepath.Join(versionsDir, e.Name(), e.Name()+".jar")
			if _, err := os.Stat(jar); err == nil {
				ids = append(ids, e.Name())
			}
		}
	}
	return ids, nil
}

func (m *Manager) RemoveVersion(id string) error {
	versionDir := filepath.Join(m.MinecraftDir, "versions", id)
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		return fmt.Errorf("version %s not installed", id)
	}
	return os.RemoveAll(versionDir)
}

func (m *Manager) InstallVersion(ctx context.Context, id string) (err error) {
	defer close(m.Progress)

	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	v, err := m.FetchVersion(id)
	if err != nil {
		return err
	}

	versionDir := filepath.Join(m.MinecraftDir, "versions", id)
	os.MkdirAll(versionDir, 0755)
	os.MkdirAll(filepath.Join(m.MinecraftDir, "libraries"), 0755)
	os.MkdirAll(filepath.Join(m.MinecraftDir, "assets", "objects"), 0755)

	report := func(task string, completed, total int) {
		pct := 0.0
		if total > 0 {
			pct = float64(completed) / float64(total) * 100
		}
		m.Progress <- types.ProgressInfo{
			Task:      task,
			Completed: completed,
			Total:     total,
			Percent:   pct,
		}
	}

	checkCtx := func() error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			return nil
		}
	}

	report("downloads.version-json", 0, 1)
	if err := checkCtx(); err != nil {
		return err
	}
	if err := m.downloadVersionJSON(v, id); err != nil {
		return fmt.Errorf("version json: %w", err)
	}
	report("downloads.version-json", 1, 1)

	report("downloads.client-jar", 0, 1)
	if err := checkCtx(); err != nil {
		return err
	}
	if err := m.downloadClient(v, id); err != nil {
		return fmt.Errorf("client jar: %w", err)
	}
	report("downloads.client-jar", 1, 1)

	libs := filterApplicableLibraries(v.Libraries)
	report("downloads.libraries", 0, len(libs))
	dlCount := 0
	for _, lib := range libs {
		if err := checkCtx(); err != nil {
			return err
		}
		if err := m.downloadLibrary(lib); err != nil {
			return fmt.Errorf("library %s: %w", lib.Name, err)
		}
		dlCount++
		report("downloads.libraries", dlCount, len(libs))
	}

	natives := filterNatives(v.Libraries)
	report("downloads.natives", 0, len(natives))
	for i, lib := range natives {
		if err := checkCtx(); err != nil {
			return err
		}
		if err := m.extractNatives(lib, versionDir); err != nil {
			return fmt.Errorf("natives %s: %w", lib.Name, err)
		}
		report("downloads.natives", i+1, len(natives))
	}

	if v.AssetIndex != nil {
		if err := checkCtx(); err != nil {
			return err
		}
		report("downloads.asset-index", 0, 1)
		if err := m.downloadAssetIndex(v.AssetIndex); err != nil {
			return fmt.Errorf("asset index: %w", err)
		}
		report("downloads.asset-index", 1, 1)

		assets, err := m.loadAssetIndex(v.AssetIndex)
		if err != nil {
			return fmt.Errorf("load asset index: %w", err)
		}

		report("downloads.assets", 0, len(assets))
		m.downloadAssets(ctx, assets, func(completed, total int) {
			report("downloads.assets", completed, total)
		})
	}

	return nil
}

func (m *Manager) downloadVersionJSON(v *types.VersionInfo, id string) error {
	path := filepath.Join(m.MinecraftDir, "versions", id, id+".json")
	if fileExistsWithSHA(path, v.SHA1) {
		return nil
	}
	return m.client.downloadFile(path, v.URL, v.SHA1)
}

func (m *Manager) downloadClient(v *types.VersionInfo, id string) error {
	if v.Downloads == nil || v.Downloads.Client == nil {
		return fmt.Errorf("no client download for version %s", id)
	}
	path := filepath.Join(m.MinecraftDir, "versions", id, id+".jar")
	client := v.Downloads.Client
	if fileExistsWithSHA(path, client.SHA1) {
		return nil
	}
	return m.client.downloadFile(path, client.URL, client.SHA1)
}

func (m *Manager) EnsureLibrary(lib types.LibraryInfo, versionDir string) error {
	if err := m.downloadLibrary(lib); err != nil {
		return err
	}
	if lib.Natives != nil {
		if err := m.extractNatives(lib, versionDir); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) downloadLibrary(lib types.LibraryInfo) error {
	if lib.Downloads == nil || lib.Downloads.Artifact == nil {
		if lib.Name != "" {
			path := filepath.Join(m.MinecraftDir, "libraries", launch.LibPath(lib.Name))
			if _, err := os.Stat(path); err == nil {
				return nil
			}
			log.L().Warn("downloadLibrary: no Downloads.Artifact, cannot download", "name", lib.Name, "path", path)
			return nil
		}
		return nil
	}
	artifact := lib.Downloads.Artifact
	path := filepath.Join(m.MinecraftDir, "libraries", artifact.Path)
	if fileExistsWithSHA(path, artifact.SHA1) {
		return nil
	}
	if artifact.URL == "" {
		log.L().Warn("downloadLibrary: no URL for library", "name", lib.Name, "path", path)
		return nil
	}
	return m.client.downloadFile(path, artifact.URL, artifact.SHA1)
}

func (m *Manager) extractNatives(lib types.LibraryInfo, versionDir string) error {
	if lib.Downloads == nil || lib.Natives == nil {
		return nil
	}

	osName := strings.ToLower(runtime.GOOS)
	arch := strings.ToLower(runtime.GOARCH)

	nativeKey := ""
	if suffix, ok := lib.Natives[osName]; ok {
		nativeKey = strings.ReplaceAll(suffix, "${arch}", arch)
	}

	if nativeKey == "" {
		return nil
	}

	var classifier *types.FileInfo
	if lib.Downloads.Classifiers != nil {
		classifier = lib.Downloads.Classifiers[nativeKey]
	}
	if classifier == nil {
		return nil
	}

	jarPath := filepath.Join(m.MinecraftDir, "libraries", classifier.Path)
	nativesDir := filepath.Join(versionDir, "natives")
	os.MkdirAll(nativesDir, 0755)

	if !fileExistsWithSHA(jarPath, classifier.SHA1) {
		if err := m.client.downloadFile(jarPath, classifier.URL, classifier.SHA1); err != nil {
			return err
		}
	}

	return extractJarNatives(jarPath, nativesDir)
}

func (m *Manager) downloadAssetIndex(index *types.AssetIndexRef) error {
	path := filepath.Join(m.MinecraftDir, "assets", "indexes", index.ID+".json")
	if fileExistsWithSHA(path, index.SHA1) {
		return nil
	}
	return m.client.downloadFile(path, index.URL, index.SHA1)
}

func (m *Manager) loadAssetIndex(index *types.AssetIndexRef) ([]types.AssetInfo, error) {
	path := filepath.Join(m.MinecraftDir, "assets", "indexes", index.ID+".json")
	return parseAssetIndex(path)
}

func (m *Manager) downloadAssets(ctx context.Context, assets []types.AssetInfo, onProgress func(int, int)) {
	type task struct {
		url  string
		path string
		sha1 string
	}

	sem := make(chan struct{}, m.Workers)
	done := make(chan bool, len(assets))

	completed := 0
	total := len(assets)

	for _, a := range assets {
		select {
		case <-ctx.Done():
			return
		default:
		}

		go func(asset types.AssetInfo) {
			sem <- struct{}{}
			defer func() { <-sem; done <- true }()

			select {
			case <-ctx.Done():
				return
			default:
			}

			objDir := filepath.Join(m.MinecraftDir, "assets", "objects")
			subDir := asset.Hash[:2]
			dest := filepath.Join(objDir, subDir, asset.Hash)
			url := assetURL(asset.Hash)

			if !fileExistsWithSHA(dest, asset.Hash) {
				m.client.downloadFile(dest, url, asset.Hash)
			}
		}(a)
	}

	for range assets {
		<-done
		completed++
		onProgress(completed, total)
	}
}

func filterApplicableLibraries(libs []types.LibraryInfo) []types.LibraryInfo {
	var out []types.LibraryInfo
	for _, lib := range libs {
		if rulesAllow(lib.Rules) {
			out = append(out, lib)
		}
	}
	return out
}

func filterNatives(libs []types.LibraryInfo) []types.LibraryInfo {
	var out []types.LibraryInfo
	for _, lib := range libs {
		if lib.Natives != nil {
			if rulesAllow(lib.Rules) {
				out = append(out, lib)
			}
		}
	}
	return out
}

func rulesAllow(rules []types.Rule) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		if r.OS == nil {
			if r.Action == "allow" {
				allowed = true
			} else {
				allowed = false
			}
			continue
		}
		if matchesOS(r.OS) {
			if r.Action == "allow" {
				allowed = true
			} else {
				allowed = false
			}
		}
	}
	return allowed
}

func matchesOS(osRule *types.OSRule) bool {
	osName := strings.ToLower(runtime.GOOS)
	if osRule.Name != "" && osRule.Name != osName {
		return false
	}
	if osRule.Arch != "" {
		arch := strings.ToLower(runtime.GOARCH)
		if osRule.Arch != arch {
			return false
		}
	}
	if osRule.Version != "" {
		return false
	}
	return true
}

func fileExistsWithSHA(path, expectedSHA string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.Size() == 0 {
		return false
	}
	if expectedSHA == "" {
		return true
	}

	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()

	h := sha1.New()
	if _, err := io.Copy(h, f); err != nil {
		return false
	}
	got := hex.EncodeToString(h.Sum(nil))
	return got == expectedSHA
}

func assetURL(hash string) string {
	return fmt.Sprintf("https://resources.download.minecraft.net/%s/%s", hash[:2], hash)
}
