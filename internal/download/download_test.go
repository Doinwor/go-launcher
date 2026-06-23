package download

import (
	"archive/zip"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/offline-launcher/internal/download/types"
)

func TestFetchManifest(t *testing.T) {
	srv := newManifestServer(t)
	defer srv.Close()

	orig := manifestURL
	manifestURL = srv.URL + "/manifest"
	defer func() { manifestURL = orig }()

	client := defaultClient()
	m, err := fetchManifest(client)
	if err != nil {
		t.Fatalf("fetchManifest: %v", err)
	}
	if m.Latest.Release != "1.21" {
		t.Errorf("expected release 1.21, got %s", m.Latest.Release)
	}
	if len(m.Versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(m.Versions))
	}
}

func TestFetchVersion(t *testing.T) {
	srv := newVersionServer(t)
	defer srv.Close()

	client := defaultClient()
	v, err := fetchVersion(client, srv.URL+"/version/1.20.4")
	if err != nil {
		t.Fatalf("fetchVersion: %v", err)
	}
	if v.ID != "1.20.4" {
		t.Errorf("expected id 1.20.4, got %s", v.ID)
	}
	if v.MainClass != "net.minecraft.client.main.Main" {
		t.Errorf("expected main class, got %s", v.MainClass)
	}
}

func TestIsVersionInstalled(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 2)

	if m.IsVersionInstalled("1.20") {
		t.Error("should not be installed yet")
	}

	verDir := filepath.Join(dir, "versions", "1.20")
	os.MkdirAll(verDir, 0755)
	os.WriteFile(filepath.Join(verDir, "1.20.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(verDir, "1.20.jar"), []byte("jar"), 0644)

	if !m.IsVersionInstalled("1.20") {
		t.Error("should be installed now")
	}
}

func TestInstalledVersions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 2)

	versions, err := m.InstalledVersions()
	if err != nil {
		t.Fatalf("InstalledVersions: %v", err)
	}
	if len(versions) != 0 {
		t.Errorf("expected 0, got %d", len(versions))
	}

	for _, id := range []string{"1.20", "1.21"} {
		verDir := filepath.Join(dir, "versions", id)
		os.MkdirAll(verDir, 0755)
		os.WriteFile(filepath.Join(verDir, id+".jar"), []byte("data"), 0644)
	}

	versions, err = m.InstalledVersions()
	if err != nil {
		t.Fatalf("InstalledVersions: %v", err)
	}
	if len(versions) != 2 {
		t.Errorf("expected 2, got %d: %v", len(versions), versions)
	}
}

func TestRulesAllow(t *testing.T) {
	tests := []struct {
		name  string
		rules []types.Rule
		want  bool
	}{
		{"no rules", nil, true},
		{"empty rules", []types.Rule{}, true},
		{"allow all", []types.Rule{{Action: "allow"}}, true},
		{"disallow all", []types.Rule{{Action: "disallow"}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rulesAllow(tt.rules); got != tt.want {
				t.Errorf("rulesAllow() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterLibraries(t *testing.T) {
	libs := []types.LibraryInfo{
		{Name: "lib-all", Rules: []types.Rule{{Action: "allow"}}},
		{Name: "lib-none", Rules: []types.Rule{{Action: "disallow"}}},
		{Name: "lib-norules"},
	}

	filtered := filterApplicableLibraries(libs)
	if len(filtered) != 2 {
		t.Errorf("expected 2, got %d", len(filtered))
	}
}

func TestFilterNatives(t *testing.T) {
	libs := []types.LibraryInfo{
		{Name: "native-lib", Natives: map[string]string{"windows": "natives-windows"}, Rules: []types.Rule{{Action: "allow"}}},
		{Name: "regular-lib"},
	}
	natives := filterNatives(libs)
	if len(natives) != 1 {
		t.Errorf("expected 1 native, got %d", len(natives))
	}
}

func TestFileExistsWithSHA(t *testing.T) {
	if fileExistsWithSHA("nonexistent", "") {
		t.Error("nonexistent file should return false")
	}

	f, _ := os.CreateTemp("", "sha-test")
	content := []byte("hello")
	f.Write(content)
	f.Close()
	defer os.Remove(f.Name())

	if !fileExistsWithSHA(f.Name(), sha1Hex(content)) {
		t.Error("should match sha1")
	}

	if fileExistsWithSHA(f.Name(), "0000000000000000000000000000000000000000") {
		t.Error("should not match wrong sha1")
	}
}

func TestDownloadFile_Retry(t *testing.T) {
	attempts := 0
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts++
		count := attempts
		mu.Unlock()
		if count < 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	client := defaultClient()
	client.retries = 3
	client.baseWait = time.Millisecond

	dest := filepath.Join(os.TempDir(), "retry-test.txt")
	defer os.Remove(dest)
	os.Remove(dest)

	if err := client.downloadFile(dest, srv.URL, ""); err != nil {
		t.Fatalf("download after retry: %v", err)
	}

	data, _ := os.ReadFile(dest)
	if string(data) != "ok" {
		t.Errorf("expected ok, got %s", string(data))
	}
}

func TestDownloadFile_AllRetriesFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := defaultClient()
	client.retries = 2
	client.baseWait = time.Millisecond

	dest := filepath.Join(os.TempDir(), "retry-fail.txt")
	defer os.Remove(dest)
	os.Remove(dest)

	err := client.downloadFile(dest, srv.URL, "")
	if err == nil {
		t.Fatal("expected error after all retries exhausted")
	}
}

func TestAssetURL(t *testing.T) {
	url := assetURL("abcdef1234567890abcdef1234567890abcdef12")
	expected := "https://resources.download.minecraft.net/ab/abcdef1234567890abcdef1234567890abcdef12"
	if url != expected {
		t.Errorf("expected %s, got %s", expected, url)
	}
}

func TestParseAssetIndex(t *testing.T) {
	dir := t.TempDir()
	idxPath := filepath.Join(dir, "index.json")

	index := types.AssetIndexData{
		Objects: map[string]types.AssetObject{
			"icons/icon_16x16.png": {Hash: "aaaabbbbccccddddeeeeffff0000111122223333", Size: 1234},
			"lang/en_us.json":      {Hash: "bbbbccccddddeeeeffff00001111222233334444", Size: 567},
		},
	}

	data, _ := json.Marshal(index)
	os.WriteFile(idxPath, data, 0644)

	assets, err := parseAssetIndex(idxPath)
	if err != nil {
		t.Fatalf("parseAssetIndex: %v", err)
	}
	if len(assets) != 2 {
		t.Errorf("expected 2 assets, got %d", len(assets))
	}
}

func TestExtractJarNatives(t *testing.T) {
	dir := t.TempDir()
	jarPath := filepath.Join(dir, "natives.jar")
	destDir := filepath.Join(dir, "natives")
	os.MkdirAll(destDir, 0755)

	createTestJar(t, jarPath, map[string]string{
		"test.dll":   "dll content",
		"test.so":    "so content",
		"ignore.txt": "not a native",
	})

	runtimeOS = func() string { return "windows" }
	defer func() { runtimeOS = func() string { return "" } }()

	if err := extractJarNatives(jarPath, destDir); err != nil {
		t.Fatalf("extractJarNatives: %v", err)
	}

	if _, err := os.Stat(filepath.Join(destDir, "test.dll")); os.IsNotExist(err) {
		t.Error("test.dll should exist")
	}
	if _, err := os.Stat(filepath.Join(destDir, "test.so")); err == nil {
		t.Error("test.so should not exist on windows")
	}
	if _, err := os.Stat(filepath.Join(destDir, "ignore.txt")); err == nil {
		t.Error("ignore.txt should not exist")
	}
}

func TestProgressChannel(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 2)

	go func() {
		for range m.Progress {
		}
	}()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("data"))
	}))
	defer srv.Close()

	v := &types.VersionInfo{
		ID: "testver",
		Downloads: &types.VersionDownloads{
			Client: &types.FileInfo{URL: srv.URL + "/client", SHA1: sha1Hex([]byte("data"))},
		},
		Libraries: []types.LibraryInfo{
			{Name: "test:lib:1.0", Downloads: &types.LibraryDownloads{
				Artifact: &types.FileInfo{URL: srv.URL + "/lib", SHA1: sha1Hex([]byte("data"))},
			}},
		},
		AssetIndex: &types.AssetIndexRef{
			ID:   "test",
			SHA1: sha1Hex([]byte(`{"objects":{}}`)),
			URL:  srv.URL + "/index",
		},
	}

	m.downloadVersionJSON(v, "testver")
	m.downloadClient(v, "testver")
}

func TestInstallVersion_NoManifest(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir, 2)

	go func() {
		for range m.Progress {
		}
	}()

	err := m.InstallVersion(context.Background(), "nonexistent-version")
	if err == nil {
		t.Error("expected error for nonexistent version")
	}
}

func newManifestServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := types.VersionManifest{
			Latest: types.LatestVersions{Release: "1.21", Snapshot: "1.21-rc1"},
			Versions: []types.VersionEntry{
				{ID: "1.21", Type: "release", URL: r.URL.Scheme + "://" + r.Host + "/version/1.21"},
				{ID: "1.20.4", Type: "release", URL: r.URL.Scheme + "://" + r.Host + "/version/1.20.4"},
				{ID: "1.20.1", Type: "release", URL: r.URL.Scheme + "://" + r.Host + "/version/1.20.1"},
			},
		}
		json.NewEncoder(w).Encode(data)
	}))
}

func newVersionServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/version/")
		v := types.VersionInfo{
			ID:        id,
			Type:      "release",
			MainClass: "net.minecraft.client.main.Main",
			Downloads: &types.VersionDownloads{
				Client: &types.FileInfo{URL: r.URL.Scheme + "://" + r.Host + "/client", SHA1: "abc"},
			},
			Libraries: []types.LibraryInfo{
				{Name: "org.lwjgl:lwjgl:3.3.1",
					Downloads: &types.LibraryDownloads{
						Artifact: &types.FileInfo{URL: r.URL.Scheme + "://" + r.Host + "/lwjgl.jar",
							Path: "org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar"},
					},
				},
			},
		}
		json.NewEncoder(w).Encode(v)
	}))
}

func createTestJar(t *testing.T, path string, files map[string]string) {
	t.Helper()
	zw, err := os.Create(path)
	if err != nil {
		t.Fatalf("create jar: %v", err)
	}
	defer zw.Close()

	z := zip.NewWriter(zw)
	for name, content := range files {
		w, err := z.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		w.Write([]byte(content))
	}
	z.Close()
}
