package launch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCommand_WithVersionJSON(t *testing.T) {
	vj := testVersionJSON()
	ctx := &TokenContext{
		Username:         "Steve",
		UUID:             "550e8400-e29b-41d4-a716-446655440000",
		AccessToken:      "offline-token:test",
		UserType:         "mojang",
		VersionID:        "1.20.4",
		AssetIndex:       "1.20",
		MinecraftDir:     "/minecraft",
		AssetsDir:        "/minecraft/assets",
		NativesDir:       "/minecraft/versions/1.20.4/natives",
		Classpath:        "/minecraft/versions/1.20.4/1.20.4.jar",
		LauncherName:     "test-launcher",
		LauncherVersion:  "1.0",
		ResolutionWidth:  "854",
		ResolutionHeight: "480",
		MaxMemory:        "4096m",
		MinMemory:        "1024m",
		JavaPath:         "/usr/bin/java",
	}

	args, err := BuildCommand(ctx, nil, vj)
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}

	if args[0] != "/usr/bin/java" {
		t.Errorf("first arg should be java path, got %s", args[0])
	}

	_ = strings.Join(args, " ")
	checks := []string{
		"--username", "Steve",
		"--uuid", "550e8400-e29b-41d4-a716-446655440000",
		"--accessToken", "offline-token:test",
		"--userType", "mojang",
		"-Xmx4096m",
		"-Xms1024m",
	}

	for _, c := range checks {
		if !contains(args, c) {
			t.Errorf("expected arg %q not found", c)
		}
	}

	rel := strings.Join(args, " ")
	if !strings.Contains(rel, "net.minecraft.client.main.Main") {
		t.Error("main class not found in args")
	}
}

func TestBuildCommand_EmptyVersion(t *testing.T) {
	_, err := BuildCommand(&TokenContext{JavaPath: "/usr/bin/java"}, nil, &VersionJSON{})
	if err == nil {
		t.Error("expected error for empty version ID")
	}
}

func TestBuildCommand_NoJava(t *testing.T) {
	ctx := &TokenContext{VersionID: "1.20", Username: "Steve"}
	_, err := BuildCommand(ctx, nil, &VersionJSON{})
	if err == nil {
		t.Error("expected error for missing java")
	}
}

func TestBuildCommand_OldArgs(t *testing.T) {
	vj := &VersionJSON{
		MainClass:     "net.minecraft.client.main.Main",
		MinecraftArgs: "--username ${auth_player_name} --version ${version_name}",
	}

	ctx := &TokenContext{
		Username:    "Alex",
		VersionID:   "1.12.2",
		JavaPath:    "/usr/bin/java",
		MaxMemory:   "2g",
		MinMemory:   "512m",
		Classpath:   "/cp.jar",
		AssetsDir:   "/assets",
		NativesDir:  "/natives",
		LauncherName: "test",
	}

	args, err := BuildCommand(ctx, nil, vj)
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--username Alex") {
		t.Errorf("expected --username Alex, got %s", joined)
	}
	if !strings.Contains(joined, "--version 1.12.2") {
		t.Errorf("expected --version 1.12.2, got %s", joined)
	}
}

func TestBuildCommand_GameJvmSplit(t *testing.T) {
	vj := &VersionJSON{
		MainClass: "net.minecraft.client.main.Main",
		Arguments: &ArgsBlock{
			JVM: []interface{}{
				"-Djava.library.path=${natives_directory}",
				"-cp", "${classpath}",
				map[string]interface{}{
					"rules": []interface{}{map[string]interface{}{"action": "allow"}},
					"value": []interface{}{"-Dnet.minecraft.client=1"},
				},
			},
			Game: []interface{}{
				"--username", "${auth_player_name}",
				"--version", "${version_name}",
				map[string]interface{}{
					"rules": []interface{}{map[string]interface{}{"action": "disallow"}},
					"value": "--demo",
				},
				map[string]interface{}{
					"rules": []interface{}{map[string]interface{}{"action": "allow"}},
					"value": "--uuid",
				},
				"--accessToken", "${auth_access_token}",
			},
		},
	}

	ctx := &TokenContext{
		Username:    "Steve",
		VersionID:   "1.21",
		AccessToken: "tok",
		UUID:        "uuid-1234",
		JavaPath:    "java",
		Classpath:   "/cp",
		MaxMemory:   "2g",
		MinMemory:   "512m",
		NativesDir:  "/natives",
		LauncherName: "test",
	}

	args, err := BuildCommand(ctx, nil, vj)
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--username Steve") {
		t.Error("--username Steve not found")
	}
	if !strings.Contains(joined, "--uuid") {
		t.Error("--uuid not found (rule allowed)")
	}
	if strings.Contains(joined, "--demo") {
		t.Error("--demo should be excluded (disallowed)")
	}
	if !strings.Contains(joined, "-Djava.library.path=/natives") {
		t.Error("native path substitution failed")
	}
}

func TestReadVersionJSON(t *testing.T) {
	dir := t.TempDir()
	vj := testVersionJSON()
	data, _ := json.Marshal(vj)
	path := filepath.Join(dir, "version.json")
	os.WriteFile(path, data, 0644)

	parsed, err := ReadVersionJSON(path)
	if err != nil {
		t.Fatalf("ReadVersionJSON: %v", err)
	}
	if parsed.ID != "1.20.4" {
		t.Errorf("expected 1.20.4, got %s", parsed.ID)
	}
}

func TestReadVersionJSON_NotFound(t *testing.T) {
	_, err := ReadVersionJSON("/nonexistent/version.json")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestBuildCommand_AuthlibInjector(t *testing.T) {
	vj := testVersionJSON()
	ctx := &TokenContext{
		Username:           "InjectorUser",
		UUID:               "00000000-0000-0000-0000-000000000001",
		AccessToken:        "test-token",
		UserType:           "mojang",
		VersionID:          "1.20.4",
		JavaPath:           "/usr/bin/java",
		Classpath:          "/cp.jar",
		MaxMemory:          "2g",
		MinMemory:          "512m",
		NativesDir:         "/natives",
		LauncherName:       "test",
		AuthlibInjector:    true,
		InjectorJarPath:    "/opt/authlib-injector.jar",
		InjectorURL:        "http://localhost:25566",
	}

	args, err := BuildCommand(ctx, nil, vj)
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}

	joined := strings.Join(args, " ")
	expected := "-javaagent:/opt/authlib-injector.jar=http://localhost:25566"
	if !strings.Contains(joined, expected) {
		t.Errorf("expected javaagent %q not found in:\n%s", expected, joined)
	}
}

func TestBuildCommand_AuthlibInjector_Disabled(t *testing.T) {
	vj := testVersionJSON()
	ctx := &TokenContext{
		Username:        "OfflineUser",
		UUID:            "00000000-0000-0000-0000-000000000002",
		AccessToken:     "offline-token",
		UserType:        "offline",
		VersionID:       "1.20.4",
		JavaPath:        "/usr/bin/java",
		Classpath:       "/cp.jar",
		MaxMemory:       "2g",
		MinMemory:       "512m",
		NativesDir:      "/natives",
		LauncherName:    "test",
		AuthlibInjector: false,
	}

	args, err := BuildCommand(ctx, nil, vj)
	if err != nil {
		t.Fatalf("BuildCommand: %v", err)
	}

	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-javaagent:") {
		t.Error("javaagent should not be present when injector is disabled")
	}
}

func TestSubstituteTokens(t *testing.T) {
	tokens := map[string]string{
		"${name}": "Steve",
		"${ver}":  "1.20",
	}

	result := substituteTokens("Hello ${name}, run ${ver}", tokens)
	expected := "Hello Steve, run 1.20"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestBuildClasspath(t *testing.T) {
	libs := []LibInfo{
		{Name: "test:lib:1.0", Downloads: &LibDownloads{
			Artifact: &FileInfo{Path: "test/lib/1.0/lib-1.0.jar"},
		}},
		{Name: "other:lib:2.0", Downloads: &LibDownloads{
			Artifact: &FileInfo{Path: "other/lib/2.0/lib-2.0.jar"},
		}},
	}

	cp := BuildClasspath("/mc", libs)
	expected1 := filepath.Join("/mc", "libraries", "test/lib/1.0/lib-1.0.jar")
	expected2 := filepath.Join("/mc", "libraries", "other/lib/2.0/lib-2.0.jar")
	if !strings.Contains(cp, expected1) {
		t.Errorf("expected %q in classpath, got %q", expected1, cp)
	}
	if !strings.Contains(cp, expected2) {
		t.Errorf("expected %q in classpath, got %q", expected2, cp)
	}
}

func TestRulesAllow(t *testing.T) {
	tests := []struct {
		name  string
		rules []Rule
		want  bool
	}{
		{"no rules", nil, true},
		{"empty", []Rule{}, true},
		{"allow", []Rule{{Action: "allow"}}, true},
		{"disallow", []Rule{{Action: "disallow"}}, false},
		{"demo feature=false", []Rule{{Action: "allow", Features: map[string]bool{"is_demo_user": true}}}, false},
		{"custom res feature=true", []Rule{{Action: "allow", Features: map[string]bool{"has_custom_resolution": true}}}, true},
		{"mixed features", []Rule{{Action: "allow", Features: map[string]bool{"is_demo_user": true, "has_custom_resolution": true}}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rulesAllow(tt.rules); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFilterLibsByOS(t *testing.T) {
	libs := []LibInfo{
		{Name: "include", Rules: []Rule{{Action: "allow"}}},
		{Name: "exclude", Rules: []Rule{{Action: "disallow"}}},
		{Name: "any"},
	}
	filtered := FilterLibsByOS(libs)
	if len(filtered) != 2 {
		t.Errorf("expected 2, got %d", len(filtered))
	}
}

func TestJavaDetection(t *testing.T) {
	path := FindJava("")
	if path != "" {
		t.Logf("Java found at: %s", path)
	}
}

func TestDefaults(t *testing.T) {
	cfg := DefaultLaunchConfig()
	if cfg.MinecraftDir == "" {
		t.Error("minecraft dir should not be empty")
	}
	if cfg.MaxMemory <= 0 {
		t.Error("max memory should be positive")
	}
}

func TestLogBuffer(t *testing.T) {
	pm := NewProcessManager()
	pm.logs = []LogEntry{
		{Line: "line1", Type: "stdout"},
		{Line: "line2", Type: "stdout"},
		{Line: "line3", Type: "stderr"},
	}

	logs := pm.GetLogs(2)
	if len(logs) != 2 {
		t.Errorf("expected 2 logs, got %d", len(logs))
	}
	if logs[0].Line != "line2" {
		t.Errorf("expected line2, got %s", logs[0].Line)
	}

	all := pm.GetLogs(0)
	if len(all) != 3 {
		t.Errorf("expected 3 logs, got %d", len(all))
	}
}

func TestProcessManager_StopWithoutStart(t *testing.T) {
	pm := NewProcessManager()
	err := pm.Stop()
	if err == nil {
		t.Error("expected error when stopping without start")
	}
}

func TestProcessManager_StatusWithoutStart(t *testing.T) {
	pm := NewProcessManager()
	status := pm.Status()
	if status.Running {
		t.Error("should not be running")
	}
}

func contains(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func testVersionJSON() *VersionJSON {
	return &VersionJSON{
		ID:        "1.20.4",
		Type:      "release",
		MainClass: "net.minecraft.client.main.Main",
		Arguments: &ArgsBlock{
			JVM: []interface{}{
				"-Djava.library.path=${natives_directory}",
				"-cp", "${classpath}",
			},
			Game: []interface{}{
				"--username", "${auth_player_name}",
				"--version", "${version_name}",
				"--gameDir", "${game_directory}",
				"--assetsDir", "${assets_root}",
				"--assetIndex", "${assets_index_name}",
				"--uuid", "${auth_uuid}",
				"--accessToken", "${auth_access_token}",
				"--userType", "${user_type}",
				"--userProperties", "{}",
				"--width", "${resolution_width}",
				"--height", "${resolution_height}",
			},
		},
		Libraries: []LibInfo{
			{Name: "org.lwjgl:lwjgl:3.3.1", Downloads: &LibDownloads{
				Artifact: &FileInfo{Path: "org/lwjgl/lwjgl/3.3.1/lwjgl-3.3.1.jar"},
			}},
		},
	}
}
