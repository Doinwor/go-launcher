package launch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type TokenContext struct {
	Username           string
	UUID               string
	AccessToken        string
	UserType           string
	XUID               string
	VersionID          string
	AssetIndex         string
	MinecraftDir       string
	AssetsDir          string
	NativesDir         string
	Classpath          string
	LauncherName       string
	LauncherVersion    string
	ResolutionWidth    string
	ResolutionHeight   string
	MaxMemory          string
	MinMemory          string
	CustomJVMArgs      []string
	JavaPath           string
	AuthlibInjector    bool
	InjectorJarPath    string
	InjectorURL        string
}

func BuildCommand(ctx *TokenContext, cfg *LaunchConfig, versionJSON *VersionJSON) ([]string, error) {
	if ctx.VersionID == "" {
		return nil, fmt.Errorf("version ID is required")
	}
	if ctx.Username == "" {
		return nil, fmt.Errorf("account is required")
	}
	if ctx.JavaPath == "" {
		return nil, fmt.Errorf("java not found")
	}

	cp := ctx.Classpath
	if cp == "" {
		libs := FilterLibsByOS(versionJSON.Libraries)
		cp = BuildClasspath(ctx.MinecraftDir, libs)
		versionJar := filepath.Join(ctx.MinecraftDir, "versions", ctx.VersionID, ctx.VersionID+".jar")
		cp = versionJar + string(os.PathListSeparator) + cp
	}

	mainClass := versionJSON.MainClass
	if mainClass == "" {
		mainClass = "net.minecraft.client.main.Main"
	}

	tokens := map[string]string{
		"${auth_player_name}":     ctx.Username,
		"${auth_uuid}":            ctx.UUID,
		"${auth_access_token}":    ctx.AccessToken,
		"${auth_session}":         "token:" + ctx.AccessToken + ":" + ctx.UUID,
		"${user_type}":            ctx.UserType,
		"${version_name}":         ctx.VersionID,
		"${assets_root}":          ctx.AssetsDir,
		"${assets_index_name}":    ctx.AssetIndex,
		"${game_directory}":       ctx.MinecraftDir,
		"${natives_directory}":    ctx.NativesDir,
		"${classpath}":            cp,
		"${classpath_separator}":  string(os.PathListSeparator),
		"${launcher_name}":        ctx.LauncherName,
		"${launcher_version}":     ctx.LauncherVersion,
		"${resolution_width}":     ctx.ResolutionWidth,
		"${resolution_height}":    ctx.ResolutionHeight,
		"${version_type}":         versionJSON.Type,
		"${library_directory}":    filepath.Join(ctx.MinecraftDir, "libraries"),
		"${clientid}":             ctx.UUID,
		"${auth_xuid}":            ctx.XUID,
	}

	args := []string{ctx.JavaPath}

	args = append(args, "-Xmx"+ctx.MaxMemory)
	args = append(args, "-Xms"+ctx.MinMemory)
	args = append(args, ctx.CustomJVMArgs...)

	if ctx.AuthlibInjector && ctx.InjectorJarPath != "" && ctx.InjectorURL != "" {
		args = append(args, fmt.Sprintf("-javaagent:%s=%s", ctx.InjectorJarPath, ctx.InjectorURL))
	}

	args = append(args, extractJVMArgs(versionJSON, tokens)...)
	args = append(args, mainClass)
	args = append(args, extractGameArgs(versionJSON, tokens)...)
	return args, nil
}

func extractJVMArgs(v *VersionJSON, tokens map[string]string) []string {
	if v.Arguments != nil && len(v.Arguments.JVM) > 0 {
		return processArgList(v.Arguments.JVM, tokens)
	}

	var jvm []string
	jvm = append(jvm, substituteTokens("-Djava.library.path=${natives_directory}", tokens))
	jvm = append(jvm, "-cp")
	jvm = append(jvm, substituteTokens("${classpath}", tokens))
	return jvm
}

func extractGameArgs(v *VersionJSON, tokens map[string]string) []string {
	if v.Arguments != nil && len(v.Arguments.Game) > 0 {
		return processArgList(v.Arguments.Game, tokens)
	}

	if v.MinecraftArgs != "" {
		parts := strings.Fields(v.MinecraftArgs)
		return substituteTokensList(parts, tokens)
	}

	return []string{
		substituteTokens("--username ${auth_player_name}", tokens),
		substituteTokens("--version ${version_name}", tokens),
		substituteTokens("--gameDir ${game_directory}", tokens),
		substituteTokens("--assetsDir ${assets_root}", tokens),
		substituteTokens("--assetIndex ${assets_index_name}", tokens),
		substituteTokens("--uuid ${auth_uuid}", tokens),
		substituteTokens("--accessToken ${auth_access_token}", tokens),
		substituteTokens("--userType ${user_type}", tokens),
		substituteTokens("--userProperties {}", tokens),
		substituteTokens("--xuid ${auth_uuid}", tokens),
		substituteTokens("--width ${resolution_width}", tokens),
		substituteTokens("--height ${resolution_height}", tokens),
	}
}

func processArgList(list []interface{}, tokens map[string]string) []string {
	var out []string
	for _, item := range list {
		switch v := item.(type) {
		case string:
			out = append(out, substituteTokens(v, tokens))
		case map[string]interface{}:
			allow := true
			if rule, ok := v["rules"]; ok {
				rulesData, _ := json.Marshal(rule)
				var rules []Rule
				json.Unmarshal(rulesData, &rules)
				allow = rulesAllow(rules)
			}
			if allow {
				if val, ok := v["values"]; ok {
					switch val := val.(type) {
					case string:
						out = append(out, substituteTokens(val, tokens))
					case []interface{}:
						for _, s := range val {
							out = append(out, substituteTokens(fmt.Sprint(s), tokens))
						}
					}
				} else if val, ok := v["value"]; ok {
					switch val := val.(type) {
					case string:
						out = append(out, substituteTokens(val, tokens))
					case []interface{}:
						for _, s := range val {
							out = append(out, substituteTokens(fmt.Sprint(s), tokens))
						}
					}
				}
			}
		}
	}
	return out
}

func substituteTokens(s string, tokens map[string]string) string {
	result := s
	for k, v := range tokens {
		result = strings.ReplaceAll(result, k, v)
	}
	return result
}

func substituteTokensList(list []string, tokens map[string]string) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = substituteTokens(s, tokens)
	}
	return out
}

func ResolveVersion(mcDir string, v *VersionJSON) (*VersionJSON, error) {
	if v.InheritsFrom == "" {
		return v, nil
	}
	parentPath := filepath.Join(mcDir, "versions", v.InheritsFrom, v.InheritsFrom+".json")
	parent, err := ReadVersionJSON(parentPath)
	if err != nil {
		return nil, fmt.Errorf("resolve inheritance from %q: %w", v.InheritsFrom, err)
	}
	resolved, err := ResolveVersion(mcDir, parent)
	if err != nil {
		return nil, err
	}
	merged := *v
	if merged.MainClass == "" {
		merged.MainClass = resolved.MainClass
	}
	if merged.MinecraftArgs == "" {
		merged.MinecraftArgs = resolved.MinecraftArgs
	}
	if merged.Assets == "" {
		merged.Assets = resolved.Assets
	}
	if merged.AssetIndex == nil {
		merged.AssetIndex = resolved.AssetIndex
	}
	if merged.Downloads == nil {
		merged.Downloads = resolved.Downloads
	}
	if merged.Arguments == nil {
		merged.Arguments = resolved.Arguments
	}
	if merged.JavaVersion == nil {
		merged.JavaVersion = resolved.JavaVersion
	}
	existing := make(map[string]bool, len(merged.Libraries))
	for _, lib := range merged.Libraries {
		if lib.Name != "" {
			existing[lib.Name] = true
		}
	}
	for _, lib := range resolved.Libraries {
		if lib.Name == "" || !existing[lib.Name] {
			merged.Libraries = append(merged.Libraries, lib)
		}
	}
	merged.InheritsFrom = ""
	return &merged, nil
}

func LibPath(name string) string {
	parts := strings.Split(name, ":")
	if len(parts) < 3 {
		return ""
	}
	group := strings.ReplaceAll(parts[0], ".", "/")
	artifact := parts[1]
	version := parts[2]
	return fmt.Sprintf("%s/%s/%s/%s-%s.jar", group, artifact, version, artifact, version)
}

func LibURL(name, repoURL string) string {
	path := LibPath(name)
	if path == "" {
		return ""
	}
	base := repoURL
	if !strings.HasSuffix(base, "/") {
		base += "/"
	}
	return base + path
}

func ReadVersionJSON(path string) (*VersionJSON, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read version json: %w", err)
	}
	var v VersionJSON
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("parse version json: %w", err)
	}
	return &v, nil
}

func DefaultLaunchConfig() *LaunchConfig {
	mcDir := filepath.Join(os.Getenv("APPDATA"), ".minecraft")
	return &LaunchConfig{
		MinecraftDir: mcDir,
		JavaPath:     "",
		MinMemory:    1024,
		MaxMemory:    4096,
		WindowWidth:  854,
		WindowHeight: 480,
	}
}
