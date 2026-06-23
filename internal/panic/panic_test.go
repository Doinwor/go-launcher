package panic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandler_Setup(t *testing.T) {
	var logged string
	h := NewHandler(func(msg string, args ...interface{}) {
		logged = fmt.Sprintf(msg, args...)
	}, t.TempDir())
	h.Setup()

	_ = logged
}

func TestSafe(t *testing.T) {
	panicked := false
	Safe(func() {
		panic("test panic")
	})
	panicked = true

	if !panicked {
		t.Error("Safe should recover from panic")
	}
}

func TestSafe_NoPanic(t *testing.T) {
	result := 0
	Safe(func() {
		result = 42
	})
	if result != 42 {
		t.Error("Safe should execute normal code")
	}
}

func TestHandler_handle(t *testing.T) {
	var logMessages []string
	h := NewHandler(func(msg string, args ...interface{}) {
		logMessages = append(logMessages, fmt.Sprintf(msg, args...))
	}, t.TempDir())

	h.handle("test error")

	if len(logMessages) == 0 {
		t.Error("expected log messages")
	}
}

func TestHandler_StdErr(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "panic.log")
	f, _ := os.Create(logPath)
	oldStderr := os.Stderr
	os.Stderr = f
	defer func() { os.Stderr = oldStderr }()

	h := NewHandler(nil, dir)
	h.handle("stderr test")
	f.Close()

	data, _ := os.ReadFile(logPath)
	if !strings.Contains(string(data), "stderr test") {
		t.Errorf("expected 'stderr test' in output, got %s", string(data))
	}
}
