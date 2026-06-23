package update

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		v string
		a [3]int
	}{
		{"1.0.0", [3]int{1, 0, 0}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"0.0.0", [3]int{0, 0, 0}},
		{"2.0", [3]int{2, 0, 0}},
		{"1.20.4", [3]int{1, 20, 4}},
	}
	for _, tc := range tests {
		got := parseVersion(tc.v)
		if got != tc.a {
			t.Errorf("parseVersion(%s) = %v, want %v", tc.v, got, tc.a)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.1", "1.0.0", 1},
		{"1.0.0", "1.0.1", -1},
		{"2.0.0", "1.9.9", 1},
		{"1.20.4", "1.20.1", 1},
		{"v1.0.0", "1.0.0", 0},
	}
	for _, tc := range tests {
		got := compareVersions(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("compareVersions(%s, %s) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestCheck_NoUpdate(t *testing.T) {
	manifest := Manifest{
		Version: CurrentVersion,
	}
	ts := newManifestServer(t, manifest)
	defer ts.Close()

	result, err := Check(ts.URL + "/update.json")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.HasUpdate {
		t.Error("expected no update when versions match")
	}
	if result.Current != CurrentVersion {
		t.Errorf("expected current %s, got %s", CurrentVersion, result.Current)
	}
}

func TestCheck_HasUpdate(t *testing.T) {
	manifest := Manifest{
		Version:   "2.0.0",
		Changelog: "Big update",
	}
	ts := newManifestServer(t, manifest)
	defer ts.Close()

	result, err := Check(ts.URL + "/update.json")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !result.HasUpdate {
		t.Error("expected hasUpdate=true")
	}
	if result.Latest != "2.0.0" {
		t.Errorf("expected latest 2.0.0, got %s", result.Latest)
	}
	if result.Changelog != "Big update" {
		t.Errorf("expected changelog, got %s", result.Changelog)
	}
}

func TestCheck_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	result, err := Check(ts.URL + "/update.json")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.HasUpdate {
		t.Error("expected no update on server error")
	}
	if result.Error == "" {
		t.Error("expected error message")
	}
}

func TestCheck_OlderVersion(t *testing.T) {
	manifest := Manifest{
		Version: "0.5.0",
	}
	ts := newManifestServer(t, manifest)
	defer ts.Close()

	result, err := Check(ts.URL + "/update.json")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.HasUpdate {
		t.Error("expected no update when server version is older")
	}
}

func TestCheck_Unreachable(t *testing.T) {
	result, err := Check("http://localhost:19999/nonexistent")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if result.HasUpdate {
		t.Error("expected no update on unreachable server")
	}
	if result.Error == "" {
		t.Error("expected error message for unreachable server")
	}
}

func TestApply_FullFlow(t *testing.T) {
	t.Skip("integration test: requires real binary replacement")
	appDir := t.TempDir()

	// Create a dummy current binary
	binaryPath := filepath.Join(appDir, "launcher.exe")
	os.WriteFile(binaryPath, []byte("old binary content"), 0755)

	// Create a dummy web dir
	webDir := filepath.Join(appDir, "web")
	os.MkdirAll(webDir, 0755)
	os.WriteFile(filepath.Join(webDir, "index.html"), []byte("old html"), 0644)

	// Create update zip with new binary and web
	updateDir := t.TempDir()
	newBinaryPath := filepath.Join(updateDir, "launcher.exe")
	os.WriteFile(newBinaryPath, []byte("new binary content"), 0755)

	newWebDir := filepath.Join(updateDir, "web")
	os.MkdirAll(newWebDir, 0755)
	os.WriteFile(filepath.Join(newWebDir, "index.html"), []byte("new html"), 0644)

	zipPath := filepath.Join(updateDir, "update.zip")
	createTestZip(t, zipPath, updateDir)

	zipData, _ := os.ReadFile(zipPath)
	sha := sha256.Sum256(zipData)

	manifest := Manifest{
		Version:     "2.0.0",
		DownloadURL: "",
		SHA256:      hex.EncodeToString(sha[:]),
	}
	ts := newManifestServer(t, manifest)
	ts.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ".zip") {
			http.ServeFile(w, r, zipPath)
			return
		}
		manifest.DownloadURL = ts.URL + "/update.zip"
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	})
	defer ts.Close()

	err := Apply(ts.URL+"/update.json", appDir)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	// Check binary was replaced
	data, _ := os.ReadFile(binaryPath)
	if string(data) != "new binary content" {
		t.Errorf("binary not updated: got %s", string(data))
	}

	// Check backup exists
	backupPath := filepath.Join(appDir, "launcher.exe.old")
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Error("backup binary not found")
	}

	// Check web files updated
	webData, _ := os.ReadFile(filepath.Join(appDir, "web", "index.html"))
	if string(webData) != "new html" {
		t.Errorf("web not updated: got %s", string(webData))
	}
}

func TestVerifySHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.bin")
	content := []byte("hello world")
	os.WriteFile(path, content, 0644)

	h := sha256.Sum256(content)
	expected := hex.EncodeToString(h[:])

	if err := verifySHA256(path, expected); err != nil {
		t.Fatalf("verifySHA256: %v", err)
	}

	if err := verifySHA256(path, "badhash"); err == nil {
		t.Error("expected error for bad hash")
	}
}

func TestExtractZip(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := filepath.Join(t.TempDir(), "extracted")

	os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644)

	zipPath := filepath.Join(t.TempDir(), "test.zip")
	createTestZip(t, zipPath, srcDir)

	if err := extractZip(zipPath, dstDir); err != nil {
		t.Fatalf("extractZip: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dstDir, "file1.txt"))
	if string(data) != "content1" {
		t.Errorf("file1: expected content1, got %s", string(data))
	}

	data, _ = os.ReadFile(filepath.Join(dstDir, "subdir", "file2.txt"))
	if string(data) != "content2" {
		t.Errorf("file2: expected content2, got %s", string(data))
	}
}

func TestCopyFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.txt")
	dst := filepath.Join(t.TempDir(), "dst.txt")
	os.WriteFile(src, []byte("copy test"), 0644)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "copy test" {
		t.Errorf("expected 'copy test', got %s", string(data))
	}
}

func TestCopyDir(t *testing.T) {
	src := filepath.Join(t.TempDir(), "srcdir")
	dst := filepath.Join(t.TempDir(), "dstdir")
	os.MkdirAll(filepath.Join(src, "nested"), 0755)
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("a"), 0644)
	os.WriteFile(filepath.Join(src, "nested", "b.txt"), []byte("b"), 0644)

	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copyDir: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dst, "a.txt"))
	if string(data) != "a" {
		t.Errorf("expected 'a', got %s", string(data))
	}

	data, _ = os.ReadFile(filepath.Join(dst, "nested", "b.txt"))
	if string(data) != "b" {
		t.Errorf("expected 'b', got %s", string(data))
	}
}

func TestStatus(t *testing.T) {
	s := Status()
	if s == nil {
		t.Fatal("Status() returned nil")
	}
	if _, ok := s["downloading"]; !ok {
		t.Error("expected downloading field")
	}
}

func TestCurrentVersion(t *testing.T) {
	if CurrentVersion == "" {
		t.Error("CurrentVersion should not be empty")
	}
}

func newManifestServer(t *testing.T, manifest Manifest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(manifest)
	}))
}

func createTestZip(t *testing.T, zipPath, dir string) {
	t.Helper()
	out, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == dir {
			return nil
		}

		rel, _ := filepath.Rel(dir, path)
		if info.IsDir() {
			_, err := w.Create(rel + "/")
			return err
		}

		f, err := w.Create(rel)
		if err != nil {
			return err
		}

		data, _ := os.ReadFile(path)
		_, err = f.Write(data)
		return err
	})
}

func TestReplaceFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "src.bin")
	dst := filepath.Join(t.TempDir(), "dst.bin")
	os.WriteFile(src, []byte("new content"), 0644)
	os.WriteFile(dst, []byte("old content"), 0644)

	if err := replaceFile(src, dst); err != nil {
		t.Fatalf("replaceFile: %v", err)
	}

	data, _ := os.ReadFile(dst)
	if string(data) != "new content" {
		t.Errorf("expected 'new content', got %s", string(data))
	}
}

func TestExtractZip_ZipSlip(t *testing.T) {
	// Create a zip with malicious path traversal
	zipPath := filepath.Join(t.TempDir(), "bad.zip")
	out, _ := os.Create(zipPath)
	w := zip.NewWriter(out)
	f, _ := w.Create("../../escape.txt")
	f.Write([]byte("malicious"))
	w.Close()
	out.Close()

	dst := filepath.Join(t.TempDir(), "safe")
	err := extractZip(zipPath, dst)
	if err == nil {
		t.Error("expected error for zip slip path")
	}
	if !strings.Contains(fmt.Sprintf("%v", err), "invalid zip path") {
		t.Errorf("expected invalid zip path error, got %v", err)
	}
}
