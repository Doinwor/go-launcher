package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/offline-launcher/internal/auth"
	"github.com/offline-launcher/internal/download"
	"github.com/offline-launcher/internal/download/types"
	"github.com/offline-launcher/internal/launch"
	"github.com/offline-launcher/internal/profiles"
	"github.com/offline-launcher/internal/settings"
)

func newTestAPI(t testing.TB) *API {
	t.Helper()
	lc := &launch.LaunchConfig{MinecraftDir: t.TempDir()}
	dm := download.NewManager(lc.MinecraftDir, 4)

	return &API{
		profileStore:        profiles.NewStore(),
		authMgr:             auth.NewManager(t.TempDir() + "/accounts.json"),
		settingsMgr:         settings.NewManager(),
		launchCfg:           lc,
		procMgr:             launch.NewProcessManager(),
		downloadMgr:         dm,
		progressSubs:        make(map[string][]chan types.ProgressInfo),
		cancelFuncs:         make(map[string]context.CancelFunc),
		checkConnectivityFn: func() bool { return true },
	}
}

func TestHandleInstallVersion_MissingVersion(t *testing.T) {
	api := newTestAPI(t)
	api.manifestCache = &types.VersionManifest{
		Versions: []types.VersionEntry{
			{ID: "1.20.4", Type: "release"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/install", bytes.NewReader([]byte(`{"versionId":"1.99.0"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	api.handleInstallVersion(c)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if resp["error"] == "" {
		t.Error("expected error message")
	}
	if resp["error"] != "версия 1.99.0 не найдена. Возможно, она была удалена из манифеста." {
		t.Errorf("unexpected error message: %s", resp["error"])
	}
}

func TestHandleInstallVersion_ExistingVersion(t *testing.T) {
	api := newTestAPI(t)
	api.manifestCache = &types.VersionManifest{
		Versions: []types.VersionEntry{
			{ID: "1.20.4", Type: "release"},
		},
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/install", bytes.NewReader([]byte(`{"versionId":"1.20.4"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	api.handleInstallVersion(c)

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if resp["message"] != "installation started" {
		t.Errorf("expected installation started, got %s", resp["message"])
	}
}

func TestHandleInstallVersion_InvalidJSON(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/install", bytes.NewReader([]byte(`not json`)))
	c.Request.Header.Set("Content-Type", "application/json")

	api.handleInstallVersion(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleInstallVersion_EmptyBody(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/install", bytes.NewReader([]byte(`{}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	api.handleInstallVersion(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCancelInstall_NotFound(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/cancel/1.20.4", nil)
	c.Params = []gin.Param{{Key: "versionId", Value: "1.20.4"}}

	api.handleCancelInstall(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var resp map[string]string
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == "" {
		t.Error("expected error message")
	}
}

func TestHandleCancelInstall_EmptyParam(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/cancel/", nil)

	api.handleCancelInstall(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// TestHandleInstalledVersions_Empty проверяет, что список пуст для новой директории
func TestHandleInstalledVersions_Empty(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/versions/installed", nil)

	api.handleInstalledVersions(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Versions []installedVersionInfo `json:"versions"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Versions) != 0 {
		t.Errorf("expected empty versions list, got %d", len(resp.Versions))
	}
}

// TestCancelAllInstallations проверяет, что CancelAllInstallations отменяет все установки
func TestCancelAllInstallations(t *testing.T) {
	api := newTestAPI(t)

	// Симулируем две активные установки
	ctx1, cancel1 := context.WithCancel(context.Background())
	ctx2, cancel2 := context.WithCancel(context.Background())
	api.cancelFuncs["1.20.4"] = cancel1
	api.cancelFuncs["1.21"] = cancel2

	api.CancelAllInstallations()

	if ctx1.Err() == nil {
		t.Error("expected ctx1 to be cancelled")
	}
	if ctx2.Err() == nil {
		t.Error("expected ctx2 to be cancelled")
	}
	if len(api.cancelFuncs) != 0 {
		t.Errorf("expected cancelFuncs to be empty, got %d", len(api.cancelFuncs))
	}
}

// TestCheckConnectivity проверяет, что checkConnectivity не паникует и возвращает bool
func TestCheckConnectivity(t *testing.T) {
	result := checkConnectivity()
	// Не должен паниковать в любом случае
	t.Logf("connectivity check: %v", result)
}

// TestHandleGetVersions_WithManifestCache проверяет, что закэшированный манифест возвращается без ошибок
func TestHandleGetVersions_WithManifestCache(t *testing.T) {
	api := newTestAPI(t)
	api.manifestCache = &types.VersionManifest{
		Latest: struct {
			Release  string `json:"release"`
			Snapshot string `json:"snapshot"`
		}{Release: "1.21", Snapshot: "1.21-pre1"},
		Versions: []types.VersionEntry{
			{ID: "1.21", Type: "release"},
			{ID: "1.20.4", Type: "release"},
		},
	}
	api.manifestCachedAt = time.Now()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/versions", nil)

	api.handleGetVersions(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Versions []types.VersionEntry `json:"versions"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Versions) != 2 {
		t.Errorf("expected 2 versions, got %d", len(resp.Versions))
	}
}

// TestHandleVersionProgress_SSE проверяет, что SSE-эндпоинт возвращает правильные заголовки
func TestHandleVersionProgress_SSE(t *testing.T) {
	api := newTestAPI(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/versions/progress/1.20.4", nil).WithContext(ctx)
	c.Params = []gin.Param{{Key: "versionId", Value: "1.20.4"}}

	go func() {
		time.Sleep(10 * time.Millisecond)
		api.publishProgress("1.20.4", types.ProgressInfo{Task: "test", Percent: 50})
		time.Sleep(10 * time.Millisecond)
		api.closeProgress("1.20.4")
	}()

	api.handleVersionProgress(c)

	ct := w.Header().Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected text/event-stream, got %s", ct)
	}
}

// TestHandleGetSettings проверяет чтение настроек (должны быть значения по умолчанию)
func TestHandleGetSettings(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/settings", nil)

	api.handleGetSettings(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var s settings.AppSettings
	json.Unmarshal(w.Body.Bytes(), &s)
	if s.MinecraftDir == "" {
		t.Error("expected default minecraftDir")
	}
}

// TestHandleLaunchStop_WhenNotRunning проверяет, что остановка без запущенной игры возвращает ошибку
func TestHandleLaunchStop_WhenNotRunning(t *testing.T) {
	api := newTestAPI(t)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/launch/stop", nil)

	api.handleLaunchStop(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// BenchmarkHandleInstallVersion замерет производительность проверки версии
func BenchmarkHandleInstallVersion(b *testing.B) {
	api := newTestAPI(b)
	api.manifestCache = &types.VersionManifest{
		Versions: make([]types.VersionEntry, 100),
	}
	for i := 0; i < 100; i++ {
		api.manifestCache.Versions[i] = types.VersionEntry{
			ID:   fmt.Sprintf("1.%d", i),
			Type: "release",
		}
	}
	api.manifestCache.Versions = append(api.manifestCache.Versions, types.VersionEntry{ID: "1.99.0", Type: "release"})

	body := []byte(`{"versionId":"1.99.0"}`)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/api/versions/install", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		api.handleInstallVersion(c)
	}
}
