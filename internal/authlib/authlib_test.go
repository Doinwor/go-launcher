package authlib

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLatestVersion(t *testing.T) {
	v, err := LatestVersion()
	if err != nil {
		t.Fatalf("LatestVersion() error: %v", err)
	}
	if v == "" {
		t.Fatal("expected non-empty version")
	}
	t.Logf("latest version: %s", v)
}

func TestDownloadJar(t *testing.T) {
	dir := t.TempDir()
	if err := DownloadJar(dir); err != nil {
		t.Fatalf("DownloadJar() error: %v", err)
	}
	jar := JarPath(dir)
	if _, err := os.Stat(jar); err != nil {
		t.Fatalf("jar not found at %s: %v", jar, err)
	}
	fi, _ := os.Stat(jar)
	t.Logf("jar size: %d bytes", fi.Size())
}

func TestIsInstalled(t *testing.T) {
	dir := t.TempDir()
	if IsInstalled(dir) {
		t.Error("expected false for empty dir")
	}
	// Create empty file
	p := filepath.Join(dir, "authlib-injector.jar")
	os.WriteFile(p, []byte("test"), 0644)
	if !IsInstalled(dir) {
		t.Error("expected true after creating jar")
	}
}

func TestJarPath(t *testing.T) {
	p := JarPath("/test")
	if p != filepath.Join("/test", "authlib-injector.jar") {
		t.Errorf("unexpected path: %s", p)
	}
}
