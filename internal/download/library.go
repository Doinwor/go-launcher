package download

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func extractJarNatives(jarPath, destDir string) error {
	reader, err := zip.OpenReader(jarPath)
	if err != nil {
		return fmt.Errorf("open jar: %w", err)
	}
	defer reader.Close()

	osName := strings.ToLower(runtimeOS())
	allowedExt := nativeExts(osName)

	for _, f := range reader.File {
		if f.FileInfo().IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(f.Name))
		if !contains(allowedExt, ext) {
			continue
		}

		outPath := filepath.Join(destDir, filepath.Base(f.Name))
		if err := extractZipFile(f, outPath); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}

	return nil
}

func nativeExts(osName string) []string {
	switch osName {
	case "windows":
		return []string{".dll", ".dll.lib"}
	case "linux":
		return []string{".so", ".so.*"}
	case "darwin":
		return []string{".dylib", ".jnilib"}
	default:
		return []string{".dll", ".so", ".dylib", ".jnilib"}
	}
}

func extractZipFile(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	outFile, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, rc)
	return err
}

var runtimeOS = func() string { return runtime.GOOS }

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if strings.EqualFold(filepath.Ext(s), item) {
			return true
		}
	}
	return false
}
