package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/offline-launcher/internal/auth"
	"github.com/offline-launcher/internal/authlib"
	"github.com/offline-launcher/internal/authserver"
	"github.com/offline-launcher/internal/download"
	"github.com/offline-launcher/internal/download/types"
	"github.com/offline-launcher/internal/launch"
	"github.com/offline-launcher/internal/log"
	"github.com/offline-launcher/internal/mods"
	"github.com/offline-launcher/internal/profiles"
	"github.com/offline-launcher/internal/settings"
	"github.com/offline-launcher/internal/skin"
	"github.com/offline-launcher/internal/update"
)

type API struct {
	authMgr      *auth.Manager
	profileStore *profiles.Store
	settingsMgr  *settings.Manager
	launchCfg    *launch.LaunchConfig
	procMgr      *launch.ProcessManager
	authSrv      *authserver.AuthServer
	skinMgr      *skin.Manager
	modsMan      *mods.Manager
	modProfileMan *mods.ProfileManager
	downloadMgr  *download.Manager

	progressMu          sync.Mutex
	progressSubs        map[string][]chan types.ProgressInfo
	manifestCache       *types.VersionManifest
	manifestCachedAt    time.Time
	cancelMu            sync.Mutex
	cancelFuncs         map[string]context.CancelFunc
	checkConnectivityFn func() bool
	ShutdownCh          chan struct{}

	updateMu     sync.RWMutex
	updateResult *update.CheckResult
	updateCachedAt time.Time

}

func New(authMgr *auth.Manager, profileStore *profiles.Store, settingsMgr *settings.Manager, launchCfg *launch.LaunchConfig) *API {
	baseDir := filepath.Dir(settingsMgr.GetSettingsPath())
	sm := skin.NewManager(baseDir)
	sm.EnsureDir()

	as := authserver.New(25566)
	as.SetSkinsDir(sm.SkinsDir())

	// Clean up old account profiles ("offline" type)
	profileStore.Load()
	for id, p := range profileStore.GetProfiles() {
		if p.Type == "offline" {
			profileStore.DeleteProfile(id)
		}
	}
	profileStore.Save()

	s := settingsMgr.Get()
	if s.MinecraftDir != "" {
		launchCfg.MinecraftDir = s.MinecraftDir
	}
	if launchCfg.MinecraftDir == "" {
		launchCfg.MinecraftDir = filepath.Join(os.Getenv("APPDATA"), ".minecraft")
	}
	if err := os.MkdirAll(launchCfg.MinecraftDir, 0755); err != nil {
		// РќРµ С„Р°С‚Р°Р»СЊРЅРѕ вЂ” РїСЂРѕСЃС‚Рѕ Р»РѕРіРёСЂСѓРµРј
		fmt.Printf("warning: cannot create minecraft dir: %v\n", err)
	}
	mm := mods.NewManager(launchCfg.MinecraftDir, baseDir)
	mm.LoadState()
	if key := s.CurseForgeKey; key != "" {
		mm.SetCurseForgeKey(key)
	}

	mpm := mods.NewProfileManager(baseDir, mm)

	dm := download.NewManager(launchCfg.MinecraftDir, 4)

	api := &API{
		checkConnectivityFn: checkConnectivity,
		authMgr:      authMgr,
		profileStore: profileStore,
		settingsMgr:  settingsMgr,
		launchCfg:    launchCfg,
		procMgr:      launch.NewProcessManager(),
		authSrv:      as,
		skinMgr:      sm,
		modsMan:      mm,
		modProfileMan:     mpm,
		downloadMgr:      dm,
		progressSubs:     make(map[string][]chan types.ProgressInfo),
		manifestCachedAt: time.Time{},
		cancelFuncs:      make(map[string]context.CancelFunc),
		ShutdownCh:       make(chan struct{}, 1),
	}

	as.SetTextureProvider(func(uuid, username string) (string, bool) {
		s := api.settingsMgr.Get()
		baseURL := fmt.Sprintf("http://localhost:%d", s.AuthServerPort)
		return api.skinMgr.TexturePayload(uuid, username, baseURL)
	})

	return api
}

func (a *API) ProcessManager() *launch.ProcessManager {
	return a.procMgr
}

func (a *API) SetupRouter(staticDir string, embeddedFS fs.FS) *gin.Engine {
	r := gin.Default()
	r.Use(corsMiddleware())
	r.Use(noCacheMiddleware())

	api := r.Group("/api")
	{
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/offline/login", a.handleAuthOfflineLogin)
			authGroup.POST("/rename", a.handleAuthRename)
			authGroup.GET("/active", a.handleAuthActive)
			authGroup.GET("/list", a.handleAuthList)
			authGroup.POST("/switch", a.handleAuthSwitch)
			authGroup.POST("/logout", a.handleAuthLogout)
		}

		api.GET("/profiles", a.handleGetProfiles)
		api.GET("/profiles/:id", a.handleGetProfile)
		api.DELETE("/profiles/:id", a.handleDeleteProfile)

		launchGroup := api.Group("/launch")
		{
			launchGroup.POST("/start", a.handleLaunchStart)
			launchGroup.POST("/stop", a.handleLaunchStop)
			launchGroup.GET("/status", a.handleLaunchStatus)
			launchGroup.GET("/logs", a.handleLaunchLogs)
		}

		asGroup := api.Group("/authserver")
		{
			asGroup.POST("/start", a.handleAuthServerStart)
			asGroup.POST("/stop", a.handleAuthServerStop)
			asGroup.GET("/status", a.handleAuthServerStatus)
		}

		authlibGroup := api.Group("/authlib")
		{
			authlibGroup.GET("/status", a.handleAuthlibStatus)
			authlibGroup.POST("/download", a.handleAuthlibDownload)
		}

		api.GET("/settings", a.handleGetSettings)
		api.PUT("/settings", a.handleUpdateSettings)
		api.POST("/settings/open-folder", a.handleOpenFolder)
		api.POST("/shutdown", a.handleShutdown)

		skinGroup := api.Group("/skin")
		{
			skinGroup.POST("/upload", a.handleSkinUpload)
			skinGroup.POST("/upload-cape", a.handleSkinUploadCape)
			skinGroup.DELETE("/:uuid", a.handleSkinDelete)
			skinGroup.GET("/:uuid", a.handleSkinInfo)
		}

		modsGroup := api.Group("/mods")
		{
			modsGroup.GET("/list", a.handleModsList)
			modsGroup.POST("/install", a.handleModsInstall)
			modsGroup.POST("/toggle", a.handleModsToggle)
			modsGroup.DELETE("/:id", a.handleModsDelete)
			modsGroup.GET("/search/modrinth", a.handleModsSearchModrinth)
			modsGroup.GET("/search/curseforge", a.handleModsSearchCurseForge)
		}

		mpGroup := api.Group("/mod-profiles")
		{
			mpGroup.GET("", a.handleModProfilesList)
			mpGroup.POST("", a.handleModProfilesCreate)
			mpGroup.DELETE("/:id", a.handleModProfilesDelete)
			mpGroup.POST("/:id/activate", a.handleModProfilesActivate)
			mpGroup.POST("/deactivate", a.handleModProfilesDeactivate)
			mpGroup.PUT("/:id/mods/:modId", a.handleModProfilesAddMod)
			mpGroup.DELETE("/:id/mods/:modId", a.handleModProfilesRemoveMod)
		}

		loaderGroup := api.Group("/loader")
		{
			loaderGroup.POST("/install", a.handleLoaderInstall)
			loaderGroup.GET("/versions/:type/:mcversion", a.handleLoaderVersions)
			loaderGroup.GET("/profiles", a.handleLoaderProfiles)
		}

		updateGroup := api.Group("/update")
		{
			updateGroup.GET("/check", a.handleUpdateCheck)
			updateGroup.GET("/check-github", a.handleUpdateCheckGitHub)
			updateGroup.POST("/apply", a.handleUpdateApply)
			updateGroup.GET("/status", a.handleUpdateStatus)
		}

		versionsGroup := api.Group("/versions")
		{
			versionsGroup.GET("", a.handleGetVersions)
			versionsGroup.GET("/installed", a.handleInstalledVersions)
			versionsGroup.POST("/install", a.handleInstallVersion)
			versionsGroup.GET("/progress/:versionId", a.handleVersionProgress)
			versionsGroup.POST("/cancel/:versionId", a.handleCancelInstall)
			versionsGroup.DELETE("/:versionId", a.handleDeleteVersion)
		}

	}

	r.Static("/skin", a.skinMgr.SkinsDir())

	if embeddedFS != nil {
		r.StaticFS("/web", http.FS(embeddedFS))
	} else if staticDir != "" {
		r.Static("/web", staticDir)
	}

	r.GET("/", func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		if embeddedFS != nil {
			c.Redirect(http.StatusTemporaryRedirect, "/web/index.html")
		} else {
			c.File(filepath.Join(staticDir, "index.html"))
		}
	})

	return r
}

func noCacheMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/web/") {
			c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
			c.Header("Pragma", "no-cache")
			c.Header("Expires", "0")
		}
		c.Next()
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// checkConnectivity РїСЂРѕРІРµСЂСЏРµС‚ РґРѕСЃС‚СѓРї Рє Mojang API СЃ С‚Р°Р№РјР°СѓС‚РѕРј 3 СЃРµРєСѓРЅРґС‹
func checkConnectivity() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", "launchermeta.mojang.com:443")
	if err != nil {
		log.L().Warn("internet connectivity check failed", "error", err.Error())
		return false
	}
	conn.Close()
	return true
}

type offlineLoginReq struct {
	Username string `json:"username" binding:"required"`
}

func (a *API) handleAuthOfflineLogin(c *gin.Context) {
	var req offlineLoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	acc, err := a.authMgr.CreateAccount(req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, acc)
}

type renameReq struct {
	Username string `json:"username" binding:"required"`
}

func (a *API) handleAuthRename(c *gin.Context) {
	var req renameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
		return
	}

	acc := a.authMgr.GetActiveAccount()
	if acc == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "нет активного аккаунта"})
		return
	}

	updated, err := a.authMgr.RenameAccount(acc.UUID, req.Username)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, updated)
}

func (a *API) handleAuthActive(c *gin.Context) {
	acc := a.authMgr.GetActiveAccount()
	if acc == nil {
		c.JSON(http.StatusOK, gin.H{"account": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"account": acc})
}

func toDownloadLibInfo(lib launch.LibInfo) types.LibraryInfo {
	dl := types.LibraryInfo{
		Name:    lib.Name,
		Natives: lib.Natives,
	}
	if lib.Downloads != nil && lib.Downloads.Artifact != nil {
		dl.Downloads = &types.LibraryDownloads{
			Artifact: &types.FileInfo{
				SHA1: lib.Downloads.Artifact.SHA1,
				Size: lib.Downloads.Artifact.Size,
				URL:  lib.Downloads.Artifact.URL,
				Path: lib.Downloads.Artifact.Path,
			},
		}
		if lib.Downloads.Classifiers != nil {
			dl.Downloads.Classifiers = make(map[string]*types.FileInfo)
			for k, v := range lib.Downloads.Classifiers {
				dl.Downloads.Classifiers[k] = &types.FileInfo{
					SHA1: v.SHA1,
					Size: v.Size,
					URL:  v.URL,
					Path: v.Path,
				}
			}
		}
	} else if lib.Name != "" && lib.URL != "" {
		path := launch.LibPath(lib.Name)
		dl.Downloads = &types.LibraryDownloads{
			Artifact: &types.FileInfo{
				URL:  launch.LibURL(lib.Name, lib.URL),
				Path: path,
			},
		}
	}
	if lib.Rules != nil {
		dl.Rules = make([]types.Rule, len(lib.Rules))
		for i, r := range lib.Rules {
			rule := types.Rule{Action: r.Action}
			if r.OS != nil {
				rule.OS = &types.OSRule{
					Name:    r.OS.Name,
					Version: r.OS.Version,
					Arch:    r.OS.Arch,
				}
			}
			dl.Rules[i] = rule
		}
	}
	return dl
}

func (a *API) handleAuthList(c *gin.Context) {
	accounts := a.authMgr.ListAccounts()
	activeUUID := a.authMgr.ActiveUUID()
	c.JSON(http.StatusOK, gin.H{
		"accounts":   accounts,
		"activeUUID": activeUUID,
	})
}

type authSwitchReq struct {
	UUID string `json:"uuid" binding:"required"`
}

func (a *API) handleAuthSwitch(c *gin.Context) {
	var req authSwitchReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	if err := a.authMgr.SwitchAccount(req.UUID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	acc := a.authMgr.GetActiveAccount()
	c.JSON(http.StatusOK, gin.H{"message": "switched", "account": acc})
}

func (a *API) handleAuthLogout(c *gin.Context) {
	if err := a.authMgr.ClearActive(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "logged out"})
}

func (a *API) handleGetProfiles(c *gin.Context) {
	a.profileStore.Load()
	allProfiles := a.profileStore.GetProfiles()
	// Filter out account profiles (type "offline")
	filtered := make(map[string]profiles.Profile)
	for id, p := range allProfiles {
		if p.Type != "offline" {
			filtered[id] = p
		}
	}
	c.JSON(http.StatusOK, filtered)
}

func (a *API) handleGetProfile(c *gin.Context) {
	id := c.Param("id")
	a.profileStore.Load()
	p, ok := a.profileStore.GetProfile(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}
	c.JSON(http.StatusOK, p)
}

func (a *API) handleDeleteProfile(c *gin.Context) {
	id := c.Param("id")
	a.profileStore.Load()
	a.profileStore.DeleteProfile(id)
	if err := a.profileStore.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "profile deleted"})
}

type launchStartReq struct {
	VersionID string `json:"versionId" binding:"required"`
}

func (a *API) handleLaunchStart(c *gin.Context) {
	var req launchStartReq
	if err := c.ShouldBindJSON(&req); err != nil {
		log.L().Warn("launch: missing versionId")
		c.JSON(http.StatusBadRequest, gin.H{"error": "РЅРµ СѓРєР°Р·Р°РЅ ID РІРµСЂСЃРёРё"})
		return
	}

	log.L().Info("launch requested", "version", req.VersionID)

	acc := a.authMgr.GetActiveAccount()
	if acc == nil {
		log.L().Warn("launch: no active account")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "РЅРµС‚ Р°РєС‚РёРІРЅРѕРіРѕ Р°РєРєР°СѓРЅС‚Р°. Р’РѕР№РґРёС‚Рµ РІ СЃРёСЃС‚РµРјСѓ."})
		return
	}

	s := a.settingsMgr.Get()

	mcDir := s.MinecraftDir
	if mcDir == "" {
		mcDir = a.launchCfg.MinecraftDir
	}

	versionDir := filepath.Join(mcDir, "versions", req.VersionID)
	versionJSONPath := filepath.Join(versionDir, req.VersionID+".json")

	versionJSON, err := launch.ReadVersionJSON(versionJSONPath)
	if err != nil {
		log.L().Error("launch: version not installed", "version", req.VersionID, "path", versionJSONPath, "error", err)
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("РІРµСЂСЃРёСЏ %s РЅРµ СѓСЃС‚Р°РЅРѕРІР»РµРЅР°", req.VersionID)})
		return
	}
	log.L().Info("version JSON loaded", "version", req.VersionID)

	if versionJSON.InheritsFrom != "" {
		parentID := versionJSON.InheritsFrom
		resolved, err := launch.ResolveVersion(mcDir, versionJSON)
		if err != nil {
			log.L().Warn("failed to resolve version inheritance", "from", parentID, "error", err)
		} else {
			versionJSON = resolved
			log.L().Info("version inheritance resolved", "parent", parentID)
		}
	}

	javaMajor := 17
	if versionJSON.JavaVersion != nil && versionJSON.JavaVersion.MajorVersion > 0 {
		javaMajor = int(versionJSON.JavaVersion.MajorVersion)
	}

	log.L().Info("searching for Java", "userPath", s.JavaPath, "minVersion", javaMajor)
	javaPath := launch.FindJavaByVersion(s.JavaPath, javaMajor)
	if javaPath == "" {
		log.L().Info("Java not found, attempting auto-install", "version", javaMajor)
		appDir := filepath.Dir(a.settingsMgr.GetSettingsPath())
		installed, installErr := launch.InstallJava(javaMajor, appDir)
		if installErr != nil {
			log.L().Error("failed to auto-install Java", "version", javaMajor, "error", installErr.Error())
			c.JSON(http.StatusPreconditionFailed, gin.H{
				"error":       fmt.Sprintf("Java %d+ РЅРµ РЅР°Р№РґРµРЅ. РђРІС‚РѕСѓСЃС‚Р°РЅРѕРІРєР° РЅРµ СѓРґР°Р»Р°СЃСЊ: %s", javaMajor, installErr.Error()),
				"message":     fmt.Sprintf("РЈСЃС‚Р°РЅРѕРІРёС‚Рµ Java %d+ РІСЂСѓС‡РЅСѓСЋ РґР»СЏ Р·Р°РїСѓСЃРєР° Minecraft %s", javaMajor, req.VersionID),
				"downloadUrl": launch.SuggestJavaDownload(),
			})
			return
		}
		log.L().Info("Java auto-installed", "path", installed)
		javaPath = installed
	}
	log.L().Info("Java found", "path", javaPath, "version", javaMajor)

	appLibs := launch.FilterLibsByOS(versionJSON.Libraries)
	for _, lib := range appLibs {
		dlLib := toDownloadLibInfo(lib)
		if err := a.downloadMgr.EnsureLibrary(dlLib, versionDir); err != nil {
			log.L().Warn("failed to ensure library", "lib", lib.Name, "error", err)
		}
	}

	nativesDir := filepath.Join(versionDir, "natives")
	assetsDir := filepath.Join(mcDir, "assets")
	assetIndex := req.VersionID
	if versionJSON.AssetIndex != nil && versionJSON.AssetIndex.ID != "" {
		assetIndex = versionJSON.AssetIndex.ID
	}

	libs := launch.FilterLibsByOS(versionJSON.Libraries)
	cp := launch.BuildClasspath(mcDir, libs)
	versionJar := filepath.Join(versionDir, req.VersionID+".jar")
	cp = versionJar + string(filepath.ListSeparator) + cp

	windowW := s.WindowWidth
	windowH := s.WindowHeight
	if windowW <= 0 {
		windowW = a.launchCfg.WindowWidth
	}
	if windowH <= 0 {
		windowH = a.launchCfg.WindowHeight
	}

	injectorEnabled := s.UseAuthlibInjector
	injectorJarPath := ""
	injectorURL := s.AuthlibInjectorURL

	if injectorEnabled && a.authSrv.IsRunning() {
		injectorURL = fmt.Sprintf("http://localhost:%d", a.authSrv.Port())
	}

	if injectorEnabled {
		appDir := filepath.Dir(a.settingsMgr.GetSettingsPath())
		injectorJarPath = authlib.JarPath(appDir)
		if _, err := os.Stat(injectorJarPath); os.IsNotExist(err) {
			log.L().Info("authlib-injector.jar not found, downloading...")
			if dlErr := authlib.DownloadJar(appDir); dlErr != nil {
				log.L().Error("failed to download authlib-injector, disabling", "error", dlErr.Error())
				injectorEnabled = false
				injectorJarPath = ""
			} else {
				log.L().Info("authlib-injector downloaded automatically")
		}
	}
}

	userType := acc.UserType
	if injectorEnabled {
		userType = "mojang"
	}

	ctx := &launch.TokenContext{
		Username:           acc.Username,
		UUID:               acc.UUID,
		AccessToken:        acc.AccessToken,
		UserType:           userType,
		XUID:               acc.XUID,
		VersionID:          req.VersionID,
		AssetIndex:         assetIndex,
		MinecraftDir:       a.launchCfg.MinecraftDir,
		AssetsDir:          assetsDir,
		NativesDir:         nativesDir,
		Classpath:          cp,
		LauncherName:       "offline-launcher",
		LauncherVersion:    "1.0.0",
		ResolutionWidth:    fmt.Sprintf("%d", windowW),
		ResolutionHeight:   fmt.Sprintf("%d", windowH),
		MaxMemory:          fmt.Sprintf("%dm", s.MaxMemory),
		MinMemory:          fmt.Sprintf("%dm", s.MinMemory),
		CustomJVMArgs:      []string{},
		JavaPath:           javaPath,
		AuthlibInjector:    injectorEnabled,
		InjectorJarPath:    injectorJarPath,
		InjectorURL:        injectorURL,
	}

	cmdArgs, err := launch.BuildCommand(ctx, a.launchCfg, versionJSON)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	pid, err := a.procMgr.Start(javaPath, cmdArgs, a.launchCfg.MinecraftDir)
	if err != nil {
		log.L().Error("launch: failed to start process", "version", req.VersionID, "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ Р·Р°РїСѓСЃС‚РёС‚СЊ Minecraft: " + err.Error()})
		return
	}

	log.L().Info("game launched successfully", "version", req.VersionID, "pid", pid)
	c.JSON(http.StatusOK, gin.H{
		"pid":     pid,
		"message": "game launched",
		"success": true,
	})
}

func (a *API) handleLaunchStop(c *gin.Context) {
	if err := a.procMgr.Stop(); err != nil {
		log.L().Warn("launch stop: process not running", "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "РёРіСЂР° РЅРµ Р·Р°РїСѓС‰РµРЅР°: " + err.Error()})
		return
	}
	log.L().Info("game process stopped")
	c.JSON(http.StatusOK, gin.H{"message": "РёРіСЂР° РѕСЃС‚Р°РЅРѕРІР»РµРЅР°"})
}

func (a *API) handleLaunchStatus(c *gin.Context) {
	status := a.procMgr.Status()
	if !status.Running && status.ExitCode != 0 && !status.ManuallyStopped {
		logs := a.procMgr.GetLogs(500)
		status.CrashAdvice = launch.AnalyzeCrash(status.ExitCode, logs)
	}
	c.JSON(http.StatusOK, status)
}

func (a *API) handleLaunchLogs(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	limitStr := c.DefaultQuery("limit", "100")
	limit, _ := strconv.Atoi(limitStr)

	logs := a.procMgr.GetLogs(limit)
	for _, entry := range logs {
		fmt.Fprintf(c.Writer, "data: {\"line\":%q,\"type\":%q,\"time\":%d}\n\n", entry.Line, entry.Type, entry.Time)
		c.Writer.Flush()
	}

	ch := a.procMgr.Subscribe()
	defer a.procMgr.Unsubscribe(ch)

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			return
		case entry, ok := <-ch:
			if !ok {
				fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
				c.Writer.Flush()
				return
			}
			fmt.Fprintf(c.Writer, "data: {\"line\":%q,\"type\":%q,\"time\":%d}\n\n", entry.Line, entry.Type, entry.Time)
			c.Writer.Flush()
		}
	}
}

func (a *API) handleAuthServerStart(c *gin.Context) {
	if a.authSrv.IsRunning() {
		c.JSON(http.StatusOK, gin.H{"message": "auth server already running", "port": a.authSrv.Port()})
		return
	}

	s := a.settingsMgr.Get()
	port := s.AuthServerPort
	if port <= 0 {
		port = 25566
	}

	if err := a.authSrv.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "auth server started", "port": port})
}

func (a *API) handleAuthServerStop(c *gin.Context) {
	if err := a.authSrv.Stop(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "auth server stopped"})
}

func (a *API) handleAuthServerStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"running": a.authSrv.IsRunning(),
		"port":    a.authSrv.Port(),
	})
}

func (a *API) handleAuthlibStatus(c *gin.Context) {
	appDir := filepath.Dir(a.settingsMgr.GetSettingsPath())
	installed := authlib.IsInstalled(appDir)
	version := ""
	if installed {
		v, err := authlib.LatestVersion()
		if err == nil {
			version = v
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"installed": installed,
		"version":   version,
		"path":      authlib.JarPath(appDir),
	})
}

func (a *API) handleAuthlibDownload(c *gin.Context) {
	appDir := filepath.Dir(a.settingsMgr.GetSettingsPath())

	if err := authlib.DownloadJar(appDir); err != nil {
		log.L().Error("authlib-injector download failed", "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ СЃРєР°С‡Р°С‚СЊ authlib-injector: " + err.Error()})
		return
	}

	log.L().Info("authlib-injector downloaded successfully")
	c.JSON(http.StatusOK, gin.H{
		"message": "authlib-injector СЃРєР°С‡Р°РЅ",
		"path":    authlib.JarPath(appDir),
	})
}

type versionedSettings struct {
	settings.AppSettings
	LauncherVersion      string `json:"launcherVersion"`
	UpdateAvailable      bool   `json:"updateAvailable"`
	UpdateLatestVersion  string `json:"updateLatestVersion"`
}

func (a *API) cachedUpdateCheck() (bool, string) {
	a.updateMu.RLock()
	if a.updateResult != nil && time.Since(a.updateCachedAt) < 5*time.Minute {
		r := a.updateResult
		a.updateMu.RUnlock()
		return r.HasUpdate, r.Latest
	}
	a.updateMu.RUnlock()

	a.updateMu.Lock()
	defer a.updateMu.Unlock()

	// Double-check after acquiring write lock
	if a.updateResult != nil && time.Since(a.updateCachedAt) < 5*time.Minute {
		return a.updateResult.HasUpdate, a.updateResult.Latest
	}

	result, err := update.CheckGitHub()
	if err != nil {
		log.L().Warn("update check failed", "error", err)
		return false, ""
	}
	a.updateResult = result
	a.updateCachedAt = time.Now()
	return result.HasUpdate, result.Latest
}

func (a *API) handleGetSettings(c *gin.Context) {
	s := a.settingsMgr.Get()
	hasUpdate, latestVer := a.cachedUpdateCheck()
	c.JSON(http.StatusOK, versionedSettings{
		AppSettings:          *s,
		LauncherVersion:      update.CurrentVersion,
		UpdateAvailable:      hasUpdate,
		UpdateLatestVersion:  latestVer,
	})
}

func (a *API) handleUpdateSettings(c *gin.Context) {
	var s settings.AppSettings
	if err := c.ShouldBindJSON(&s); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if s.CurseForgeKey != "" {
		a.modsMan.SetCurseForgeKey(s.CurseForgeKey)
	}

	if s.MinecraftDir == "" {
		s.MinecraftDir = filepath.Join(os.Getenv("APPDATA"), ".minecraft")
	}

	// РЎРѕР·РґР°С‚СЊ РїР°РїРєСѓ .minecraft, РµСЃР»Рё РµС‘ РЅРµС‚
	if err := os.MkdirAll(s.MinecraftDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ СЃРѕР·РґР°С‚СЊ РїР°РїРєСѓ .minecraft: " + err.Error()})
		return
	}

	if err := a.settingsMgr.Update(&s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if err := a.settingsMgr.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	a.launchCfg.MinecraftDir = s.MinecraftDir
	a.downloadMgr.MinecraftDir = s.MinecraftDir
	a.modsMan.SetMinecraftDir(s.MinecraftDir)

	c.JSON(http.StatusOK, gin.H{"message": "settings saved"})
}

func (a *API) handleOpenFolder(c *gin.Context) {
	mcDir := a.launchCfg.MinecraftDir
	if mcDir == "" {
		mcDir = filepath.Join(os.Getenv("APPDATA"), ".minecraft")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", mcDir)
	case "darwin":
		cmd = exec.Command("open", mcDir)
	default:
		cmd = exec.Command("xdg-open", mcDir)
	}
	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ РѕС‚РєСЂС‹С‚СЊ РїР°РїРєСѓ: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "folder opened"})
}

func (a *API) handleModsList(c *gin.Context) {
	modsList, err := a.modsMan.Scan()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	activeProfile := a.modProfileMan.ActiveID()
	var profileModIDs []string
	if activeProfile != "" {
		profileModIDs = a.modProfileMan.ModIDsInProfile(activeProfile)
	}

	c.JSON(http.StatusOK, gin.H{
		"mods":           modsList,
		"activeProfile":  activeProfile,
		"profileModIDs":  profileModIDs,
	})
}

func (a *API) handleModsInstall(c *gin.Context) {
	var req mods.InstallModReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	mod, err := a.modsMan.Install(req.ProjectID, req.VersionID, req.URL, req.GameVersion, "", req.Loader, req.Source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	a.modProfileMan.AddModToActive(mod.FileName)

	c.JSON(http.StatusOK, gin.H{"mod": mod, "message": "mod installed"})
}

type modsToggleReq struct {
	ID string `json:"id" binding:"required"`
}

func (a *API) handleModsToggle(c *gin.Context) {
	var req modsToggleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	mod, err := a.modsMan.Toggle(req.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	status := "disabled"
	if mod.Enabled {
		status = "enabled"
	}

	added, _ := a.modProfileMan.ToggleModInActive(mod.FileName)

	c.JSON(http.StatusOK, gin.H{"mod": mod, "message": "mod " + status, "profileAdded": added})
}

func (a *API) handleModsDelete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	if err := a.modsMan.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "mod deleted"})
}

func (a *API) handleModProfilesList(c *gin.Context) {
	profiles := a.modProfileMan.List()
	activeID := a.modProfileMan.ActiveID()
	c.JSON(http.StatusOK, gin.H{"profiles": profiles, "activeProfile": activeID})
}

type createModProfileReq struct {
	Name string `json:"name" binding:"required"`
}

func (a *API) handleModProfilesCreate(c *gin.Context) {
	var req createModProfileReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	p, err := a.modProfileMan.Create(req.Name)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "profile already exists"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"profile": p, "message": "profile created"})
}

func (a *API) handleModProfilesDelete(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	if err := a.modProfileMan.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "profile deleted"})
}

func (a *API) handleModProfilesActivate(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	a.modProfileMan.EnsureVersionProfile(id)

	if err := a.modProfileMan.Activate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "profile activated"})
}

func (a *API) handleModProfilesDeactivate(c *gin.Context) {
	if err := a.modProfileMan.Deactivate(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "profile deactivated"})
}

func (a *API) handleModProfilesAddMod(c *gin.Context) {
	profileID := c.Param("id")
	modID := c.Param("modId")
	if profileID == "" || modID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id and modId are required"})
		return
	}

	if err := a.modProfileMan.AddMod(profileID, modID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "mod added to profile"})
}

func (a *API) handleModProfilesRemoveMod(c *gin.Context) {
	profileID := c.Param("id")
	modID := c.Param("modId")
	if profileID == "" || modID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id and modId are required"})
		return
	}

	if err := a.modProfileMan.RemoveMod(profileID, modID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "profile not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "mod removed from profile"})
}

func (a *API) handleModsSearchModrinth(c *gin.Context) {
	query := c.Query("query")
	gameVersion := c.Query("gameVersion")
	loader := c.Query("loader")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	result, err := a.modsMan.SearchModrinth(query, gameVersion, mods.LoaderType(loader), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (a *API) handleModsSearchCurseForge(c *gin.Context) {
	query := c.Query("query")
	gameVersion := c.Query("gameVersion")
	loader := c.Query("loader")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	result, err := a.modsMan.SearchCurseForge(query, gameVersion, mods.LoaderType(loader), limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (a *API) handleLoaderInstall(c *gin.Context) {
	var req mods.InstallLoaderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	s := a.settingsMgr.Get()
	javaPath := launch.FindJava(s.JavaPath)
	if javaPath == "" {
		c.JSON(http.StatusPreconditionFailed, gin.H{
			"error":       "Java 17+ not found",
			"message":     "РЈСЃС‚Р°РЅРѕРІРёС‚Рµ Java 17+",
			"downloadUrl": launch.SuggestJavaDownload(),
		})
		return
	}

	mcDir := s.MinecraftDir
	resp, err := mods.InstallLoader(req, mcDir, javaPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp.JavaPath = javaPath
	a.modProfileMan.EnsureVersionProfile(resp.ProfileID)

	c.JSON(http.StatusOK, gin.H{
		"message":     fmt.Sprintf("%s %s СѓСЃС‚Р°РЅРѕРІР»РµРЅ РґР»СЏ Minecraft %s", mods.LoaderDisplayName(req.Loader), resp.LoaderVersion, req.MCVersion),
		"result":      resp,
	})
}

func (a *API) handleLoaderVersions(c *gin.Context) {
	loaderType := mods.LoaderType(c.Param("type"))
	mcVersion := c.Param("mcversion")

	var versions []mods.LoaderVersion
	var err error

	switch loaderType {
	case mods.LoaderFabric:
		metas, e := mods.FetchFabricVersions(mcVersion)
		err = e
		if err == nil {
			for _, m := range metas {
				versions = append(versions, mods.LoaderVersion{
					Loader:  mods.LoaderFabric,
					Version: m.Loader.Version,
					Stable:  m.Loader.Stable,
				})
			}
		}
	case mods.LoaderQuilt:
		metas, e := mods.FetchQuiltVersions(mcVersion)
		err = e
		if err == nil {
			for _, m := range metas {
				versions = append(versions, mods.LoaderVersion{
					Loader:  mods.LoaderQuilt,
					Version: m.Loader.Version,
					Stable:  m.Loader.Stable,
				})
			}
		}
	case mods.LoaderForge:
		metas, e := mods.FetchForgeVersions(mcVersion)
		err = e
		if err == nil {
			for _, m := range metas {
				versions = append(versions, mods.LoaderVersion{
					Loader:  mods.LoaderForge,
					Version: m.Version,
				})
			}
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported loader: " + string(loaderType)})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (a *API) handleLoaderProfiles(c *gin.Context) {
	s := a.settingsMgr.Get()
	profiles, err := mods.InstalledProfiles(s.MinecraftDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	type profileInfo struct {
		ID     string        `json:"id"`
		Loader mods.LoaderType `json:"loader"`
	}

	var result []profileInfo
	for _, p := range profiles {
		result = append(result, profileInfo{
			ID:     p,
			Loader: mods.DetectLoader(p),
		})
	}

	c.JSON(http.StatusOK, gin.H{"profiles": result})
}

func (a *API) handleSkinUpload(c *gin.Context) {
	uuid := c.PostForm("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	file, _, err := c.Request.FormFile("skin")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "skin file is required"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	if err := a.skinMgr.SaveSkin(uuid, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "skin saved",
		"uuid":      uuid,
		"skin_path": a.skinMgr.SkinPath(uuid),
	})
}

func (a *API) handleSkinUploadCape(c *gin.Context) {
	uuid := c.PostForm("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	file, _, err := c.Request.FormFile("cape")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cape file is required"})
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read file"})
		return
	}

	if err := a.skinMgr.SaveCape(uuid, data); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "cape saved",
		"uuid":      uuid,
		"cape_path": a.skinMgr.CapePath(uuid),
	})
}

func (a *API) handleSkinDelete(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	a.skinMgr.DeleteSkin(uuid)
	a.skinMgr.DeleteCape(uuid)

	c.JSON(http.StatusOK, gin.H{"message": "skin and cape deleted"})
}

func (a *API) handleSkinInfo(c *gin.Context) {
	uuid := c.Param("uuid")
	if uuid == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "uuid is required"})
		return
	}

	hasSkin := a.skinMgr.HasSkin(uuid)
	hasCape := a.skinMgr.HasCape(uuid)

	resp := gin.H{
		"uuid":    uuid,
		"hasSkin": hasSkin,
		"hasCape": hasCape,
	}

	if hasSkin {
		resp["skinPath"] = a.skinMgr.SkinPath(uuid)
	}
	if hasCape {
		resp["capePath"] = a.skinMgr.CapePath(uuid)
	}

	c.JSON(http.StatusOK, resp)
}

func (a *API) handleUpdateCheckGitHub(c *gin.Context) {
	result, err := update.CheckGitHub()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *API) handleUpdateCheck(c *gin.Context) {
	manifestURL := c.DefaultQuery("url", "")
	result, err := update.Check(manifestURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (a *API) handleUpdateApply(c *gin.Context) {
	appDir := filepath.Dir(a.settingsMgr.GetSettingsPath())

	manifestURL := c.DefaultQuery("url", "")

	go func() {
		if err := update.Apply(manifestURL, appDir); err != nil {
			return
		}
		update.Restart()
	}()

	c.JSON(http.StatusOK, gin.H{"message": "update started"})
}

func (a *API) handleUpdateStatus(c *gin.Context) {
	c.JSON(http.StatusOK, update.Status())
}

func (a *API) handleGetVersions(c *gin.Context) {
	if a.manifestCache == nil || time.Since(a.manifestCachedAt) > 5*time.Minute {
		if !a.checkConnectivityFn() {
			if a.manifestCache != nil {
				log.L().Warn("no internet, using stale manifest cache")
				c.JSON(http.StatusOK, gin.H{"versions": a.manifestCache.Versions, "cached": true})
				return
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "РЅРµС‚ РїРѕРґРєР»СЋС‡РµРЅРёСЏ Рє РёРЅС‚РµСЂРЅРµС‚Сѓ. РџСЂРѕРІРµСЂСЊС‚Рµ СЃРѕРµРґРёРЅРµРЅРёРµ Рё РїРѕРІС‚РѕСЂРёС‚Рµ РїРѕРїС‹С‚РєСѓ."})
			return
		}
		log.L().Info("fetching version manifest from Mojang")
		manifest, err := a.downloadMgr.FetchManifest()
		if err != nil {
			if a.manifestCache != nil {
				log.L().Warn("failed to refresh manifest, using stale cache", "error", err.Error())
			} else {
				log.L().Error("failed to fetch version manifest", "error", err.Error())
				c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ Р·Р°РіСЂСѓР·РёС‚СЊ СЃРїРёСЃРѕРє РІРµСЂСЃРёР№. РџРѕРїСЂРѕР±СѓР№С‚Рµ РїРѕР·Р¶Рµ."})
				return
			}
		} else {
			a.manifestCache = manifest
			a.manifestCachedAt = time.Now()
			log.L().Info("manifest fetched successfully", "versions", len(manifest.Versions))
		}
	}

	c.JSON(http.StatusOK, gin.H{"versions": a.manifestCache.Versions})
}

type installedVersionInfo struct {
	ID       string        `json:"id"`
	Loader   mods.LoaderType `json:"loader"`
	Modified int64         `json:"modified"`
}

func (a *API) handleInstalledVersions(c *gin.Context) {
	mcDir := a.settingsMgr.Get().MinecraftDir
	if mcDir == "" {
		mcDir = a.launchCfg.MinecraftDir
	}

	ids, err := a.downloadMgr.InstalledVersions()
	if err != nil {
		log.L().Error("failed to list installed versions", "error", err.Error())
		c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ РїРѕР»СѓС‡РёС‚СЊ СЃРїРёСЃРѕРє СѓСЃС‚Р°РЅРѕРІР»РµРЅРЅС‹С… РІРµСЂСЃРёР№"})
		return
	}
	log.L().Info("listed installed versions", "count", len(ids))

	var versions []installedVersionInfo
	for _, id := range ids {
		dir := filepath.Join(mcDir, "versions", id)
		fi, err := os.Stat(dir)
		modTime := int64(0)
		if err == nil {
			modTime = fi.ModTime().Unix()
		}
		versions = append(versions, installedVersionInfo{
			ID:       id,
			Loader:   mods.DetectLoader(id),
			Modified: modTime,
		})
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i].Modified > versions[j].Modified
	})

	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

type installVersionReq struct {
	VersionID string `json:"versionId" binding:"required"`
}

func (a *API) handleInstallVersion(c *gin.Context) {
	var req installVersionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		log.L().Warn("install version: missing versionId in request")
		c.JSON(http.StatusBadRequest, gin.H{"error": "РЅРµ СѓРєР°Р·Р°РЅ ID РІРµСЂСЃРёРё"})
		return
	}

	log.L().Info("install version requested", "version", req.VersionID)

	if !a.checkConnectivityFn() {
		log.L().Error("install version: no internet", "version", req.VersionID)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "РЅРµС‚ РїРѕРґРєР»СЋС‡РµРЅРёСЏ Рє РёРЅС‚РµСЂРЅРµС‚Сѓ. РЈСЃС‚Р°РЅРѕРІРєР° РЅРµРІРѕР·РјРѕР¶РЅР°."})
		return
	}

	// РџСЂРѕРІРµСЂРєР°: РІРµСЂСЃРёСЏ РґРѕР»Р¶РЅР° СЃСѓС‰РµСЃС‚РІРѕРІР°С‚СЊ РІ РјР°РЅРёС„РµСЃС‚Рµ
	manifest := a.manifestCache
	if manifest == nil {
		var err error
		log.L().Info("fetching manifest for version install", "version", req.VersionID)
		manifest, err = a.downloadMgr.FetchManifest()
		if err != nil {
			log.L().Error("install version: failed to fetch manifest", "version", req.VersionID, "error", err.Error())
			c.JSON(http.StatusInternalServerError, gin.H{"error": "РЅРµ СѓРґР°Р»РѕСЃСЊ Р·Р°РіСЂСѓР·РёС‚СЊ СЃРїРёСЃРѕРє РІРµСЂСЃРёР№. РџСЂРѕРІРµСЂСЊС‚Рµ РёРЅС‚РµСЂРЅРµС‚-СЃРѕРµРґРёРЅРµРЅРёРµ."})
			return
		}
	}

	found := false
	for _, v := range manifest.Versions {
		if v.ID == req.VersionID {
			found = true
			break
		}
	}
	if !found {
		log.L().Warn("install version: version not in manifest", "version", req.VersionID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "РІРµСЂСЃРёСЏ " + req.VersionID + " РЅРµ РЅР°Р№РґРµРЅР°. Р’РѕР·РјРѕР¶РЅРѕ, РѕРЅР° Р±С‹Р»Р° СѓРґР°Р»РµРЅР° РёР· РјР°РЅРёС„РµСЃС‚Р°."})
		return
	}

	mcDir := a.settingsMgr.Get().MinecraftDir
	if mcDir == "" {
		mcDir = a.launchCfg.MinecraftDir
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelMu.Lock()
	if oldCancel, ok := a.cancelFuncs[req.VersionID]; ok {
		log.L().Info("cancelling previous installation for version", "version", req.VersionID)
		oldCancel()
	}
	a.cancelFuncs[req.VersionID] = cancel
	a.cancelMu.Unlock()

	dm := a.downloadMgr
	dm.MinecraftDir = mcDir
	dm.Progress = make(chan types.ProgressInfo, 64)

	log.L().Info("starting version installation", "version", req.VersionID, "mcDir", mcDir)

	go func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					log.L().Info("progress subscriber stopped (context cancelled)", "version", req.VersionID)
					return
				case p, ok := <-dm.Progress:
					if !ok {
						return
					}
					a.publishProgress(req.VersionID, p)
				}
			}
		}()

		log.L().Info("installing version", "version", req.VersionID)
		err := dm.InstallVersion(ctx, req.VersionID)
		if err != nil {
			if ctx.Err() != nil {
				log.L().Info("version installation cancelled", "version", req.VersionID)
				a.publishProgress(req.VersionID, types.ProgressInfo{Task: "cancelled", Percent: 0})
			} else {
				log.L().Error("version installation failed", "version", req.VersionID, "error", err.Error())
				a.publishProgress(req.VersionID, types.ProgressInfo{Task: "error", Percent: 0})
			}
		} else {
			// РЎРѕР·РґР°С‚СЊ РїСЂРѕС„РёР»СЊ РІ launcher_profiles.json
			now := time.Now().UTC().Format(time.RFC3339)
			profile := profiles.Profile{
				ID:            req.VersionID,
				Name:          req.VersionID,
				Type:          "custom",
				Created:       now,
				LastUsed:      now,
				LastVersionID: req.VersionID,
				Icon:          "Furnace",
			}
			a.profileStore.UpsertProfile(profile)
			a.profileStore.SetSelectedProfile(req.VersionID)
			a.modProfileMan.EnsureVersionProfile(req.VersionID)
			if err := a.profileStore.Save(); err != nil {
				log.L().Error("failed to save profile after version install", "version", req.VersionID, "error", err.Error())
			}
			a.publishProgress(req.VersionID, types.ProgressInfo{Task: "done", Percent: 100})
		}
		a.closeProgress(req.VersionID)
		a.cancelMu.Lock()
		delete(a.cancelFuncs, req.VersionID)
		a.cancelMu.Unlock()
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "installation started",
		"version": req.VersionID,
	})
}

func (a *API) handleCancelInstall(c *gin.Context) {
	versionID := c.Param("versionId")
	if versionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "versionId is required"})
		return
	}

	a.cancelMu.Lock()
	cancel, ok := a.cancelFuncs[versionID]
	delete(a.cancelFuncs, versionID)
	a.cancelMu.Unlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active installation for " + versionID})
		return
	}

	cancel()
	log.L().Info("installation cancelled by user", "version", versionID)
	a.publishProgress(versionID, types.ProgressInfo{Task: "cancelled", Percent: 0})
	a.closeProgress(versionID)

	c.JSON(http.StatusOK, gin.H{"message": "installation cancelled", "version": versionID})
}

func (a *API) handleDeleteVersion(c *gin.Context) {
	versionID := c.Param("versionId")
	if versionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "versionId is required"})
		return
	}

	// Delete the profile if it exists
	a.profileStore.DeleteProfile(versionID)
	a.profileStore.Save()

	if err := a.downloadMgr.RemoveVersion(versionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	log.L().Info("version deleted", "version", versionID)
	c.JSON(http.StatusOK, gin.H{"message": "version deleted", "version": versionID})
}

func (a *API) subscribeProgress(versionID string) <-chan types.ProgressInfo {
	ch := make(chan types.ProgressInfo, 16)
	a.progressMu.Lock()
	a.progressSubs[versionID] = append(a.progressSubs[versionID], ch)
	a.progressMu.Unlock()
	return ch
}

func (a *API) publishProgress(versionID string, p types.ProgressInfo) {
	a.progressMu.Lock()
	subs := a.progressSubs[versionID]
	for _, ch := range subs {
		select {
		case ch <- p:
		default:
		}
	}
	a.progressMu.Unlock()
}

func (a *API) closeProgress(versionID string) {
	a.progressMu.Lock()
	subs := a.progressSubs[versionID]
	delete(a.progressSubs, versionID)
	for _, ch := range subs {
		close(ch)
	}
	a.progressMu.Unlock()
}

func (a *API) handleShutdown(c *gin.Context) {
	log.L().Info("shutdown requested via API")
	select {
	case a.ShutdownCh <- struct{}{}:
	default:
	}
	c.JSON(http.StatusOK, gin.H{"message": "shutting down"})
}

// CancelAllInstallations РѕС‚РјРµРЅСЏРµС‚ РІСЃРµ Р°РєС‚РёРІРЅС‹Рµ СѓСЃС‚Р°РЅРѕРІРєРё (РІС‹Р·С‹РІР°РµС‚СЃСЏ РїСЂРё graceful shutdown)
func (a *API) CancelAllInstallations() {
	a.cancelMu.Lock()
	defer a.cancelMu.Unlock()
	for versionID, cancel := range a.cancelFuncs {
		log.L().Info("cancelling installation", "version", versionID)
		cancel()
		delete(a.cancelFuncs, versionID)
	}
}

func (a *API) handleVersionProgress(c *gin.Context) {
	versionID := c.Param("versionId")
	if versionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "versionId is required"})
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	ch := a.subscribeProgress(versionID)
	defer func() {
		a.progressMu.Lock()
		subs := a.progressSubs[versionID]
		for i, sub := range subs {
			if sub == ch {
				a.progressSubs[versionID] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		a.progressMu.Unlock()
	}()

	clientGone := c.Request.Context().Done()
	for {
		select {
		case <-clientGone:
			return
		case p, ok := <-ch:
			if !ok {
				fmt.Fprintf(c.Writer, "event: done\ndata: {}\n\n")
				c.Writer.Flush()
				return
			}
			data, _ := json.Marshal(p)
			fmt.Fprintf(c.Writer, "data: %s\n\n", data)
			c.Writer.Flush()
		}
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
