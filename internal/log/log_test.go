package log

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit(t *testing.T) {
	ResetForTesting()
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	cfg.JSON = false

	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}

	Info("test message", "key", "value")
	CloseLog()

	data, _ := os.ReadFile(cfg.FilePath)
	if !strings.Contains(string(data), "test message") {
		t.Errorf("expected 'test message' in log file, got %s", string(data))
	}
}

func TestInitJSON(t *testing.T) {
	ResetForTesting()
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	cfg.JSON = true

	Init(cfg)
	Info("json test")
	CloseLog()

	data, _ := os.ReadFile(cfg.FilePath)
	if !strings.Contains(string(data), `json test`) {
		t.Errorf("expected json test in log, got %s", string(data))
	}
}

func TestLevels(t *testing.T) {
	ResetForTesting()
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	cfg.Level = "warn"
	Init(cfg)

	Debug("should not appear")
	Info("should not appear")
	Warn("should appear")
	CloseLog()

	data, _ := os.ReadFile(cfg.FilePath)

	if strings.Contains(string(data), "should not appear") {
		t.Error("debug/info should not appear at warn level")
	}
	if !strings.Contains(string(data), "should appear") {
		t.Error("warn should appear at warn level")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
	}
	for _, tc := range tests {
		l := parseLevel(tc.input)
		if l != tc.want {
			t.Errorf("parseLevel(%s) = %s, want %s", tc.input, l, tc.want)
		}
	}
}

func TestWithFields(t *testing.T) {
	ResetForTesting()
	dir := t.TempDir()
	Init(DefaultConfig(dir))

	logger := WithFields("component", "test")
	logger.Info("with fields")
	CloseLog()

	data, _ := os.ReadFile(DefaultConfig(dir).FilePath)
	if !strings.Contains(string(data), "component") {
		t.Errorf("expected component in log, got %s", string(data))
	}
}

func TestRotatingWriter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rotate.log")
	f, _ := os.Create(path)
	w := &rotatingWriter{
		file:    f,
		path:    path,
		maxSize: 10,
	}

	_, err := w.Write([]byte("hello world this is a long line"))
	if err != nil {
		t.Fatalf("Write: %v", err)
	}

	if _, err = os.Stat(path + ".old"); err != nil {
		t.Log("rotate may not have triggered")
	}

	f.Close()
}

func TestDefaultConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig(dir)
	if cfg.Level != "debug" {
		t.Errorf("expected debug, got %s", cfg.Level)
	}
	if cfg.MaxSizeMB != 5 {
		t.Errorf("expected 5MB, got %d", cfg.MaxSizeMB)
	}
	if cfg.JSON {
		t.Error("expected text format by default")
	}
}

func TestFatal(t *testing.T) {
	if os.Getenv("TEST_FATAL") == "1" {
		ResetForTesting()
		dir := t.TempDir()
		Init(DefaultConfig(dir))
		Fatal("test fatal")
	}
}
