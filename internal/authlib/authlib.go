package authlib

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	githubAPI = "https://api.github.com/repos/yushijinhun/authlib-injector/releases/latest"
	owner     = "offline-launcher"
	repo      = "offline-launcher"
)

type releaseInfo struct {
	TagName string        `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

func JarPath(appDir string) string {
	return filepath.Join(appDir, "authlib-injector.jar")
}

func IsInstalled(appDir string) bool {
	_, err := os.Stat(JarPath(appDir))
	return err == nil
}

func LatestVersion() (string, error) {
	req, err := http.NewRequest("GET", githubAPI, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", owner, repo))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("parse release: %w", err)
	}

	if release.TagName == "" {
		return "", fmt.Errorf("no releases found")
	}

	return release.TagName, nil
}

func DownloadJar(appDir string) error {
	destPath := JarPath(appDir)

	if _, err := os.Stat(destPath); err == nil {
		return nil
	}

	req, err := http.NewRequest("GET", githubAPI, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", fmt.Sprintf("%s/%s", owner, repo))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetch release info: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		resp.Body.Close()
		return fmt.Errorf("parse release: %w", err)
	}
	resp.Body.Close()

	var jarURL string
	for _, a := range release.Assets {
		if a.Name == "" {
			continue
		}
		ext := filepath.Ext(a.Name)
		if ext == ".jar" {
			jarURL = a.BrowserDownloadURL
			break
		}
	}

	if jarURL == "" {
		return fmt.Errorf("no jar asset found in release %s", release.TagName)
	}

	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	defer out.Close()

	dlReq, err := http.NewRequest("GET", jarURL, nil)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("create download request: %w", err)
	}
	dlReq.Header.Set("User-Agent", fmt.Sprintf("%s/%s", owner, repo))

	dlResp, err := http.DefaultClient.Do(dlReq)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("download jar: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != http.StatusOK {
		os.Remove(tmpPath)
		return fmt.Errorf("download HTTP %d", dlResp.StatusCode)
	}

	if _, err := io.Copy(out, dlResp.Body); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("write jar: %w", err)
	}
	out.Close()

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}
