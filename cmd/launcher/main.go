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
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
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

func killProcessOnPort(port string) {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("netstat", "-ano")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out, err := cmd.Output()
		if err != nil {
			return
		}
		lines := strings.Split(string(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, ":"+port) && strings.Contains(line, "LISTENING") {
				parts := strings.Fields(line)
				if len(parts) > 4 {
					pid := parts[len(parts)-1]
					kill := exec.Command("taskkill", "/F", "/PID", pid)
					kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
					kill.Run()
				}
			}
		}
	} else {
		// Linux / macOS
		out, err := exec.Command("lsof", "-ti", ":"+port).Output()
		if err != nil {
			return
		}
		pid := strings.TrimSpace(string(out))
		if pid != "" {
			exec.Command("kill", "-9", pid).Run()
		}
	}
}

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

	var embeddedFS fs.FS
	sub, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.L().Error("failed to create embedded filesystem", "error", err)
		staticDir := findStaticDir()
		if staticDir == "" {
			os.Exit(1)
		}
		log.L().Warn("using static web directory", "path", staticDir)
	} else {
		embeddedFS = sub
		log.L().Info("using embedded web files")
	}
	router := srv.SetupRouter("", embeddedFS)

	port := "8080"
	if p := os.Getenv("LAUNCHER_PORT"); p != "" {
		port = p
	}

	// Убиваем старый процесс на этом порту, если есть
	addr := ":" + port
	if ln, err := net.Listen("tcp", addr); err != nil {
		log.L().Info("port " + port + " already in use — убиваем старый процесс")
		killProcessOnPort(port)
		// Повторяем попытку
		var retryErr error
		for i := 0; i < 5; i++ {
			var retryLn net.Listener
			retryLn, retryErr = net.Listen("tcp", addr)
			if retryErr == nil {
				retryLn.Close()
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if retryErr != nil {
			log.L().Error("port " + port + " still in use after killing process")
			os.Exit(1)
		}
	} else {
		ln.Close()
	}

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

	// РћС‚РјРµРЅРёС‚СЊ РІСЃРµ Р°РєС‚РёРІРЅС‹Рµ СѓСЃС‚Р°РЅРѕРІРєРё
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
		os.Stderr.WriteString("\nв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђ\n")
		os.Stderr.WriteString("  РџР РћРР—РћРЁР›Рђ РљР РРўРР§Р•РЎРљРђРЇ РћРЁРР‘РљРђ\n")
		os.Stderr.WriteString("в•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђ\n")
		os.Stderr.WriteString("  " + msg + "\n\n")
		os.Stderr.WriteString("  РџРѕР¶Р°Р»СѓР№СЃС‚Р°, РѕС‚РїСЂР°РІСЊС‚Рµ С„Р°Р№Р» launcher.log\n")
		os.Stderr.WriteString("  СЂР°Р·СЂР°Р±РѕС‚С‡РёРєСѓ РґР»СЏ РёСЃРїСЂР°РІР»РµРЅРёСЏ РїСЂРѕР±Р»РµРјС‹.\n")
		os.Stderr.WriteString("в•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђв•ђ\n\n")
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
