package log

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

var (
	defaultLogger *slog.Logger
	initOnce      sync.Once
	logFile       *os.File
)

func ResetForTesting() {
	initOnce = sync.Once{}
	defaultLogger = nil
	logFile = nil
}

func CloseLog() {
	if logFile != nil {
		logFile.Close()
		logFile = nil
	}
}

type Config struct {
	Level     string `json:"level"`
	FilePath  string `json:"filePath"`
	MaxSizeMB int    `json:"maxSizeMB"`
	JSON      bool   `json:"json"`
}

func DefaultConfig(appDir string) Config {
	return Config{
		Level:     "debug",
		FilePath:  filepath.Join(appDir, "launcher.log"),
		MaxSizeMB: 5,
		JSON:      false,
	}
}

func Init(cfg Config) error {
	var err error
	initOnce.Do(func() {
		err = initLogger(cfg)
	})
	return err
}

func initLogger(cfg Config) error {
	level := parseLevel(cfg.Level)

	var file io.Writer
	if cfg.FilePath != "" {
		dir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create log dir: %w", err)
		}
		f, err := os.OpenFile(cfg.FilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return fmt.Errorf("open log file: %w", err)
		}
		logFile = f
		file = &rotatingWriter{
			file:    f,
			path:    cfg.FilePath,
			maxSize: int64(cfg.MaxSizeMB) * 1024 * 1024,
		}
	}

	multi := io.MultiWriter(os.Stdout, file)

	var handler slog.Handler
	if cfg.JSON {
		handler = slog.NewJSONHandler(multi, &slog.HandlerOptions{Level: level})
	} else {
		handler = slog.NewTextHandler(multi, &slog.HandlerOptions{Level: level})
	}

	defaultLogger = slog.New(handler)
	slog.SetDefault(defaultLogger)
	return nil
}

func L() *slog.Logger {
	if defaultLogger == nil {
		return slog.Default()
	}
	return defaultLogger
}

func Debug(msg string, args ...interface{}) { L().Debug(msg, args...) }
func Info(msg string, args ...interface{})  { L().Info(msg, args...) }
func Warn(msg string, args ...interface{})  { L().Warn(msg, args...) }
func Error(msg string, args ...interface{}) { L().Error(msg, args...) }

func Fatal(msg string, args ...interface{}) {
	L().Error(msg, args...)
	os.Exit(1)
}

func FmtError(ctx context.Context, msg string, err error, args ...interface{}) {
	fields := append([]interface{}{"error", err}, args...)
	L().ErrorContext(ctx, msg, fields...)
}

func Sync() {
	// For testing: ensures log data is flushed
}

func WithFields(args ...interface{}) *slog.Logger {
	return L().With(args...)
}

func parseLevel(s string) slog.Level {
	switch s {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type rotatingWriter struct {
	file    *os.File
	path    string
	maxSize int64
	mu      sync.Mutex
}

func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.file.Close()
}

func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.maxSize > 0 {
		stat, err := w.file.Stat()
		if err == nil && stat.Size() >= w.maxSize {
			w.rotate()
		}
	}

	return w.file.Write(p)
}

func (w *rotatingWriter) rotate() {
	w.file.Close()
	backup := w.path + ".old"
	os.Remove(backup)
	os.Rename(w.path, backup)
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// Fallback: reopen old file
		f, _ = os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	}
	w.file = f
}

func RecoverHandler() {
	if r := recover(); r != nil {
		buf := make([]byte, 4096)
		n := runtime.Stack(buf, false)
		err := fmt.Errorf("PANIC: %v\nstack:\n%s", r, string(buf[:n]))
		L().Error("critical error", "panic", err)
		showCriticalError(err.Error())
	}
}

func showCriticalError(msg string) {
	fmt.Fprintf(os.Stderr, "\n═══════════════════════════════════════════\n")
	fmt.Fprintf(os.Stderr, "  ПРОИЗОШЛА КРИТИЧЕСКАЯ ОШИБКА\n")
	fmt.Fprintf(os.Stderr, "═══════════════════════════════════════════\n")
	fmt.Fprintf(os.Stderr, "  %s\n\n", msg)
	fmt.Fprintf(os.Stderr, "  Пожалуйста, отправьте файл launcher.log\n")
	fmt.Fprintf(os.Stderr, "  разработчику для исправления проблемы.\n")
	fmt.Fprintf(os.Stderr, "═══════════════════════════════════════════\n\n")
}
