package update

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

var CurrentVersion = "1.0.8"

const GitHubReleasesURL = "https://api.github.com/repos/Doinwor/go-launcher/releases/latest"

var DefaultManifestURL = "https://your-update-server.com/update.json"

type Manifest struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
	SHA256      string `json:"sha256"`
	Changelog   string `json:"changelog"`
	MinVersion  string `json:"minVersion,omitempty"`
}

type CheckResult struct {
	HasUpdate bool      `json:"hasUpdate"`
	Current   string    `json:"current"`
	Latest    string    `json:"latest"`
	Changelog string    `json:"changelog,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type UpdateStatus struct {
	mu          sync.RWMutex
	downloading bool
	progress    float64
	message     string
	done        bool
	err         string
}

var status = &UpdateStatus{}

func Status() map[string]interface{} {
	status.mu.RLock()
	defer status.mu.RUnlock()
	return map[string]interface{}{
		"downloading": status.downloading,
		"progress":    status.progress,
		"message":     status.message,
		"done":        status.done,
		"error":       status.err,
	}
}

func CheckGitHub() (*CheckResult, error) {
	result, err := checkGitHubAPI()
	if err == nil {
		return result, nil
	}
	// Fallback: try gh-pages version.json
	fallback, fbErr := checkVersionJSON()
	if fbErr == nil {
		return fallback, nil
	}
	return nil, err
}

func checkGitHubAPI() (*CheckResult, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", GitHubReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "go-launcher/"+CurrentVersion)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub HTTP %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
		Body    string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parse release: %w", err)
	}

	hasUpdate := compareVersions(strings.TrimPrefix(release.TagName, "v"), CurrentVersion) > 0

	result := &CheckResult{
		HasUpdate: hasUpdate,
		Current:   CurrentVersion,
		Latest:    strings.TrimPrefix(release.TagName, "v"),
	}
	if hasUpdate {
		result.Changelog = release.Body
	}

	return result, nil
}

var versionJSONURLs = []string{
	"https://doinwor.github.io/go-launcher/version.json",
}

func checkVersionJSON() (*CheckResult, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	for _, url := range versionJSONURLs {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var v struct {
			Latest string `json:"latest"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
			continue
		}
		if v.Latest == "" {
			continue
		}

		hasUpdate := compareVersions(v.Latest, CurrentVersion) > 0
		return &CheckResult{
			HasUpdate: hasUpdate,
			Current:   CurrentVersion,
			Latest:    v.Latest,
		}, nil
	}
	return nil, fmt.Errorf("no version.json reachable")
}

func Check(manifestURL string) (*CheckResult, error) {
	if manifestURL == "" {
		manifestURL = DefaultManifestURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(manifestURL)
	if err != nil {
		return &CheckResult{
			HasUpdate: false,
			Current:   CurrentVersion,
			Latest:    CurrentVersion,
			Error:     fmt.Sprintf("update server unreachable: %v", err),
		}, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &CheckResult{
			HasUpdate: false,
			Current:   CurrentVersion,
			Latest:    CurrentVersion,
			Error:     fmt.Sprintf("update server HTTP %d", resp.StatusCode),
		}, nil
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	hasUpdate := compareVersions(manifest.Version, CurrentVersion) > 0

	result := &CheckResult{
		HasUpdate: hasUpdate,
		Current:   CurrentVersion,
		Latest:    manifest.Version,
	}
	if hasUpdate {
		result.Changelog = manifest.Changelog
	}

	return result, nil
}

func Apply(manifestURL string, appDir string) error {
	status.mu.Lock()
	status.downloading = true
	status.progress = 0
	status.message = "Р—Р°РіСЂСѓР·РєР° РјР°РЅРёС„РµСЃС‚Р°..."
	status.done = false
	status.err = ""
	status.mu.Unlock()

	defer func() {
		status.mu.Lock()
		status.downloading = false
		status.mu.Unlock()
	}()

	if manifestURL == "" {
		manifestURL = DefaultManifestURL
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(manifestURL)
	if err != nil {
		setError(fmt.Sprintf("manifest request: %v", err))
		return fmt.Errorf("manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		setError(fmt.Sprintf("manifest HTTP %d", resp.StatusCode))
		return fmt.Errorf("manifest HTTP %d", resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		setError(fmt.Sprintf("parse manifest: %v", err))
		return fmt.Errorf("parse manifest: %w", err)
	}

	setProgress(0.1, fmt.Sprintf("РќРѕРІР°СЏ РІРµСЂСЃРёСЏ: %s", manifest.Version))

	tmpDir := filepath.Join(os.TempDir(), "offline-launcher-update")
	os.RemoveAll(tmpDir)
	os.MkdirAll(tmpDir, 0755)

	zipPath := filepath.Join(tmpDir, "update.zip")
	setProgress(0.15, "РЎРєР°С‡РёРІР°РЅРёРµ РѕР±РЅРѕРІР»РµРЅРёСЏ...")

	if err := downloadFile(zipPath, manifest.DownloadURL); err != nil {
		setError(fmt.Sprintf("download: %v", err))
		return fmt.Errorf("download: %w", err)
	}

	setProgress(0.6, "РџСЂРѕРІРµСЂРєР° С†РµР»РѕСЃС‚РЅРѕСЃС‚Рё...")

	if manifest.SHA256 != "" {
		if err := verifySHA256(zipPath, manifest.SHA256); err != nil {
			setError(fmt.Sprintf("SHA256 mismatch: %v", err))
			return fmt.Errorf("sha256: %w", err)
		}
	}

	setProgress(0.7, "Р Р°СЃРїР°РєРѕРІРєР°...")

	extractDir := filepath.Join(tmpDir, "extracted")
	if err := extractZip(zipPath, extractDir); err != nil {
		setError(fmt.Sprintf("extract: %v", err))
		return fmt.Errorf("extract: %w", err)
	}

	setProgress(0.85, "РЈСЃС‚Р°РЅРѕРІРєР°...")

	if err := applyUpdate(extractDir, appDir); err != nil {
		setError(fmt.Sprintf("apply: %v", err))
		return err
	}

	setProgress(1.0, "Р“РѕС‚РѕРІРѕ. РџРµСЂРµР·Р°РїСѓСЃРє...")

	status.mu.Lock()
	status.done = true
	status.mu.Unlock()

	return nil
}

func Restart() error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "start", "", execPath)
	} else {
		cmd = exec.Command(execPath)
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("restart: %w", err)
	}

	os.Exit(0)
	return nil
}

func applyUpdate(extractDir, appDir string) error {
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("executable path: %w", err)
	}

	execName := filepath.Base(execPath)
	execDir := filepath.Dir(execPath)

	backupPath := filepath.Join(execDir, execName+".old")

	// Find the binary in extracted dir
	extractedBin := filepath.Join(extractDir, execName)
	if _, err := os.Stat(extractedBin); os.IsNotExist(err) {
		// Try without path
		entries, _ := filepath.Glob(filepath.Join(extractDir, "*.exe"))
		if len(entries) > 0 {
			extractedBin = entries[0]
		}
		entries, _ = filepath.Glob(filepath.Join(extractDir, "*"))
		for _, e := range entries {
			if !strings.HasSuffix(e, ".zip") && !strings.HasSuffix(e, ".old") {
				info, err := os.Stat(e)
				if err == nil && !info.IsDir() && info.Mode()&0111 != 0 {
					extractedBin = e
					break
				}
			}
		}
	}

	// Backup current binary
	os.Remove(backupPath)
	if err := copyFile(execPath, backupPath); err != nil {
		return fmt.Errorf("backup binary: %w", err)
	}

	// Replace binary
	if err := replaceFile(extractedBin, execPath); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}

	// Update static files (web/)
	extractedWeb := filepath.Join(extractDir, "web")
	if info, err := os.Stat(extractedWeb); err == nil && info.IsDir() {
		webDir := filepath.Join(execDir, "web")
		webBackup := filepath.Join(execDir, "web.old")
		os.RemoveAll(webBackup)
		if info, err := os.Stat(webDir); err == nil && info.IsDir() {
			if err := os.Rename(webDir, webBackup); err != nil {
				// Non-fatal: continue with new web files
			}
		}
		if err := copyDir(extractedWeb, webDir); err != nil {
			return fmt.Errorf("update web files: %w", err)
		}
		os.RemoveAll(webBackup)
	}

	// Clean up temp
	os.RemoveAll(filepath.Join(os.TempDir(), "offline-launcher-update"))

	return nil
}

func compareVersions(a, b string) int {
	an := parseVersion(a)
	bn := parseVersion(b)
	for i := 0; i < 3; i++ {
		if an[i] > bn[i] {
			return 1
		}
		if an[i] < bn[i] {
			return -1
		}
	}
	return 0
}

func parseVersion(v string) [3]int {
	var res [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		fmt.Sscanf(parts[i], "%d", &res[i])
	}
	return res
}

func downloadFile(destPath, url string) error {
	dir := filepath.Dir(destPath)
	os.MkdirAll(dir, 0755)

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	buf := make([]byte, 32*1024)
	var written int64
	total := resp.ContentLength

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return werr
			}
			written += int64(n)
			if total > 0 {
				setProgress(0.15+0.45*(float64(written)/float64(total)), "РЎРєР°С‡РёРІР°РЅРёРµ РѕР±РЅРѕРІР»РµРЅРёСЏ...")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	return nil
}

func verifySHA256(path, expected string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	h := sha256.Sum256(data)
	got := hex.EncodeToString(h[:])
	if got != strings.ToLower(expected) {
		return fmt.Errorf("expected %s, got %s", expected, got)
	}
	return nil
}

func extractZip(src, dest string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(dest, f.Name)

		if !strings.HasPrefix(filepath.Clean(fpath), filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid zip path: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			os.MkdirAll(fpath, 0755)
			continue
		}

		os.MkdirAll(filepath.Dir(fpath), 0755)

		rc, err := f.Open()
		if err != nil {
			return err
		}

		out, err := os.OpenFile(fpath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func replaceFile(src, dst string) error {
	os.Remove(dst)
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()

	info, err := s.Stat()
	if err != nil {
		return err
	}

	d, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer d.Close()

	_, err = io.Copy(d, s)
	return err
}

func copyDir(src, dst string) error {
	os.MkdirAll(dst, 0755)

	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func setProgress(p float64, msg string) {
	status.mu.Lock()
	status.progress = p
	status.message = msg
	status.mu.Unlock()
}

func setError(msg string) {
	status.mu.Lock()
	status.err = msg
	status.downloading = false
	status.mu.Unlock()
}
