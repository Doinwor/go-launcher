package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/offline-launcher/internal/auth"
	"github.com/offline-launcher/internal/installid"
	"github.com/offline-launcher/internal/launch"
	"github.com/offline-launcher/internal/log"
	"github.com/offline-launcher/internal/open"
	"github.com/offline-launcher/internal/profiles"
	"github.com/offline-launcher/internal/server"
	"github.com/offline-launcher/internal/settings"
)

//go:embed web/*
var webFiles embed.FS

var version = "dev"

func main() {
	defer recoverPanic()

	appDir := getAppDir()
	os.MkdirAll(appDir, 0755)

	if err := log.Init(log.DefaultConfig(appDir)); err != nil {
		os.Stderr.WriteString("failed to init logger: " + err.Error() + "\n")
	}
	log.L().Info("launcher starting", "version", version, "appDir", appDir)

	installID := installid.New(appDir)
	log.L().Info("installation ID", "id", installID.Get()[:12]+"...")

	authMgr := auth.NewManager(filepath.Join(appDir, "accounts.json"))

	profileStore := profiles.NewStore()
	profileStore.SetFilename(filepath.Join(appDir, "launcher_profiles.json"))
	profileStore.Load()

	settingsMgr := settings.NewManager()
	settingsMgr.Load()

	launchCfg := launch.DefaultLaunchConfig()

	srv := server.New(authMgr, profileStore, settingsMgr, launchCfg)

	staticDir := findStaticDir()
	var embeddedFS fs.FS
	if staticDir == "" {
		sub, err := fs.Sub(webFiles, "web")
		if err != nil {
			log.L().Error("failed to create embedded filesystem", "error", err)
			os.Exit(1)
		}
		embeddedFS = sub
	}
	router := srv.SetupRouter(staticDir, embeddedFS)

	port := "8080"
	if p := os.Getenv("LAUNCHER_PORT"); p != "" {
		port = p
	}

	// Проверка: не занят ли порт другим лаунчером
	addr := ":" + port
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.L().Error("port " + port + " already in use — возможно, лаунчер уже запущен")
		fmt.Fprintf(os.Stderr, "ОШИБКА: порт %s уже занят. Возможно, лаунчер уже запущен.\n", port)
		fmt.Fprintf(os.Stderr, "Завершите предыдущий процесс или укажите другой порт через LAUNCHER_PORT.\n")
		os.Exit(1)
	}
	ln.Close()

	httpSrv := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		log.L().Info("listening", "port", port, "url", "http://localhost:"+port)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.L().Error("server error", "error", err)
		}
	}()

	url := "http://localhost:" + port
	if err := open.Browser(url); err != nil {
		log.L().Warn("auto-open browser failed", "error", err)
	} else {
		log.L().Info("browser opened", "url", url)
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	var shutdownCause string
	select {
	case sig := <-quit:
		shutdownCause = fmt.Sprintf("signal: %s", sig.String())
	case <-srv.ShutdownCh:
		shutdownCause = "api request"
	}
	log.L().Info("shutting down", "cause", shutdownCause)

	// Отменить все активные установки
	log.L().Info("cancelling active installations")
	srv.CancelAllInstallations()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(ctx); err != nil {
		log.L().Error("server shutdown error", "error", err)
	}

	if srv.ProcessManager() != nil {
		log.L().Info("stopping game process")
		srv.ProcessManager().Stop()
	}

	log.L().Info("launcher stopped")
}

func recoverPanic() {
	if r := recover(); r != nil {
		msg := "critical error"
		if err, ok := r.(error); ok {
			msg = err.Error()
		}
		log.L().Error("FATAL PANIC", "panic", r, "stack", string(debug.Stack()))
		os.Stderr.WriteString("\n═══════════════════════════════════════════\n")
		os.Stderr.WriteString("  ПРОИЗОШЛА КРИТИЧЕСКАЯ ОШИБКА\n")
		os.Stderr.WriteString("═══════════════════════════════════════════\n")
		os.Stderr.WriteString("  " + msg + "\n\n")
		os.Stderr.WriteString("  Пожалуйста, отправьте файл launcher.log\n")
		os.Stderr.WriteString("  разработчику для исправления проблемы.\n")
		os.Stderr.WriteString("═══════════════════════════════════════════\n\n")
		os.Exit(1)
	}
}

func getAppDir() string {
	base := os.Getenv("APPDATA")
	if base == "" {
		base = os.Getenv("HOME")
		if base == "" {
			base = "."
		}
	}
	dir := filepath.Join(base, "offline-launcher")
	return dir
}

func findStaticDir() string {
	candidates := []string{"web", "../web", filepath.Join("..", "web")}
	for _, d := range candidates {
		info, err := os.Stat(d)
		if err == nil && info.IsDir() {
			index := filepath.Join(d, "index.html")
			if _, err := os.Stat(index); err == nil {
				return d
			}
		}
	}
	return ""
}
