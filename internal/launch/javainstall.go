package launch

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type adoptiumAsset struct {
	Binary struct {
		Package struct {
			Link string `json:"link"`
			Name string `json:"name"`
		} `json:"package"`
	} `json:"binary"`
}

func JavaDir(appDir string, version int) string {
	return filepath.Join(appDir, "runtimes", fmt.Sprintf("java-%d", version))
}

func javaExePath(javaDir string) string {
	return filepath.Join(javaDir, "bin", "java.exe")
}

func IsJavaInstalled(appDir string, version int) bool {
	_, err := os.Stat(javaExePath(JavaDir(appDir, version)))
	return err == nil
}

func InstallJava(version int, appDir string) (string, error) {
	destDir := JavaDir(appDir, version)
	javaExe := javaExePath(destDir)

	if _, err := os.Stat(javaExe); err == nil {
		setJavaCacheWithVersion(javaExe, version)
		return javaExe, nil
	}

	os.MkdirAll(destDir, 0755)

	url, err := getAdoptiumDownloadURL(version)
	if err != nil {
		return "", fmt.Errorf("get download url: %w", err)
	}

	zipPath := filepath.Join(destDir, fmt.Sprintf("java-%d.zip", version))

	if err := downloadFile(zipPath, url); err != nil {
		os.Remove(zipPath)
		return "", fmt.Errorf("download java: %w", err)
	}

	if err := extractZip(zipPath, destDir); err != nil {
		os.Remove(zipPath)
		return "", fmt.Errorf("extract java: %w", err)
	}

	os.Remove(zipPath)

	entries, err := os.ReadDir(destDir)
	if err != nil {
		return "", fmt.Errorf("read runtimes dir: %w", err)
	}

	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(strings.ToLower(e.Name()), "jdk-") {
			src := filepath.Join(destDir, e.Name(), "bin", "java.exe")
			if _, err := os.Stat(src); err == nil {
				os.Rename(filepath.Join(destDir, e.Name()), filepath.Join(destDir, "jdk"))
				break
			}
		}
	}

	javaDir := filepath.Join(destDir, "jdk")
	javaExe = filepath.Join(javaDir, "bin", "java.exe")

	if _, err := os.Stat(javaExe); err != nil {
		for _, e := range entries {
			if e.IsDir() {
				alt := filepath.Join(destDir, e.Name(), "bin", "java.exe")
				if _, err := os.Stat(alt); err == nil {
					javaExe = alt
					break
				}
			}
		}
	}

	if _, err := os.Stat(javaExe); err != nil {
		return "", fmt.Errorf("java executable not found after extraction")
	}

	setJavaCacheWithVersion(javaExe, version)
	return javaExe, nil
}

func getAdoptiumDownloadURL(version int) (string, error) {
	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "x64"
	}
	apiURL := fmt.Sprintf("https://api.adoptium.net/v3/assets/latest/%d/hotspot?architecture=%s&os=windows&image_type=jdk", version, arch)

	resp, err := http.Get(apiURL)
	if err != nil {
		return "", fmt.Errorf("request adoptium api: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("adoptium api returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read adoptium response: %w", err)
	}

	var assets []adoptiumAsset
	if err := json.Unmarshal(body, &assets); err != nil {
		return "", fmt.Errorf("parse adoptium response: %w", err)
	}

	if len(assets) == 0 {
		return "", fmt.Errorf("no adoptium assets found for Java %d", version)
	}

	link := assets[0].Binary.Package.Link
	if link == "" {
		return "", fmt.Errorf("empty download link in adoptium response")
	}

	return link, nil
}

func downloadFile(path, url string) error {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		fpath := filepath.Join(destDir, f.Name)

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
