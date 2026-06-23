package launch

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/offline-launcher/internal/log"
)

var (
	javaCache        string
	javaCacheVersion int
	javaCacheMu      sync.RWMutex
)

func FindJava(userPath string) string {
	return FindJavaByVersion(userPath, 17)
}

func FindJavaByVersion(userPath string, minVersion int) string {
	javaCacheMu.RLock()
	cached := javaCache
	javaCacheMu.RUnlock()

	if cached != "" && javaCacheVersion >= minVersion {
		if _, err := os.Stat(cached); err == nil {
			log.L().Debug("using cached java path", "path", cached)
			return cached
		}
		log.L().Debug("cached java path no longer valid, rescanning", "path", cached)
	}

	if userPath != "" {
		log.L().Debug("checking user-provided java path", "path", userPath)
		if _, err := os.Stat(userPath); err == nil {
			ver, err := CheckJavaVersion(userPath)
			if err == nil && ver >= minVersion {
				log.L().Info("java found (user path)", "path", userPath, "version", ver)
				setJavaCacheWithVersion(userPath, ver)
				return userPath
			}
			log.L().Debug("user java path version too old or invalid", "path", userPath, "version", ver)
		}
	}

	if jh := os.Getenv("JAVA_HOME"); jh != "" {
		javaExe := filepath.Join(jh, "bin", "java")
		if runtime.GOOS == "windows" {
			javaExe += ".exe"
		}
		log.L().Debug("checking JAVA_HOME", "path", javaExe)
		if _, err := os.Stat(javaExe); err == nil {
			if ver, err := CheckJavaVersion(javaExe); err == nil && ver >= minVersion {
				log.L().Info("java found (JAVA_HOME)", "path", javaExe, "version", ver)
				setJavaCacheWithVersion(javaExe, ver)
				return javaExe
			}
		}
	}

	for _, p := range defaultJavaPaths() {
		log.L().Debug("checking default java path", "path", p)
		if _, err := os.Stat(p); err == nil {
			if ver, err := CheckJavaVersion(p); err == nil && ver >= minVersion {
				log.L().Info("java found (default path)", "path", p, "version", ver)
				setJavaCacheWithVersion(p, ver)
				return p
			}
		}
	}

	if p := checkPathJava(); p != "" {
		log.L().Debug("checking PATH java", "path", p)
		if ver, err := CheckJavaVersion(p); err == nil && ver >= minVersion {
			log.L().Info("java found (PATH)", "path", p, "version", ver)
			setJavaCacheWithVersion(p, ver)
			return p
		}
	}

	log.L().Warn(fmt.Sprintf("java %d+ not found after scanning all locations", minVersion))
	return ""
}

func setJavaCache(path string) {
	setJavaCacheWithVersion(path, 0)
}

func setJavaCacheWithVersion(path string, ver int) {
	javaCacheMu.Lock()
	javaCache = path
	javaCacheVersion = ver
	javaCacheMu.Unlock()
}

func ClearJavaCache() {
	javaCacheMu.Lock()
	javaCache = ""
	javaCacheMu.Unlock()
}

func defaultJavaPaths() []string {
	if runtime.GOOS != "windows" {
		return []string{
			"/usr/lib/jvm/java-21-openjdk/bin/java",
			"/usr/lib/jvm/java-17-openjdk/bin/java",
			"/usr/lib/jvm/java-11-openjdk/bin/java",
			"/usr/lib/jvm/java-8-openjdk/bin/java",
			"/usr/bin/java",
			"/usr/local/bin/java",
		}
	}

	var candidateDirs []string
	appData := os.Getenv("APPDATA")
	if appData != "" {
		runtimesDir := filepath.Join(appData, "offline-launcher", "runtimes")
		candidateDirs = append(candidateDirs, runtimesDir)
	}
	for _, base := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
		if base == "" {
			continue
		}
		for _, vendor := range []string{
			"Java",
			"Eclipse Adoptium",
			"AdoptOpenJDK",
			"Amazon Corretto",
			"BellSoft",
			"Zulu",
			"Microsoft",
			"Liberica JDK",
			"ojdkbuild",
			"SAPMachine",
			"GraalVM",
		} {
			candidateDirs = append(candidateDirs, filepath.Join(base, vendor))
		}
	}

	javaHome := os.Getenv("JAVA_HOME")
	if javaHome != "" {
		candidateDirs = append(candidateDirs, javaHome)
	}

	var paths []string
	seen := map[string]bool{}

	var walkDir func(dir string)
	walkDir = func(dir string) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			javaExe := filepath.Join(dir, "bin", "java.exe")
			if _, err := os.Stat(javaExe); err == nil && !seen[javaExe] {
				seen[javaExe] = true
				paths = append(paths, javaExe)
			}
			return
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			javaExe := filepath.Join(dir, e.Name(), "bin", "java.exe")
			if _, err := os.Stat(javaExe); err == nil && !seen[javaExe] {
				seen[javaExe] = true
				paths = append(paths, javaExe)
			} else {
				walkDir(filepath.Join(dir, e.Name()))
			}
		}
	}
	for _, dir := range candidateDirs {
		walkDir(dir)
	}

	return paths
}

func checkPathJava() string {
	java := "java"
	if runtime.GOOS == "windows" {
		java = "java.exe"
	}
	p, err := exec.LookPath(java)
	if err != nil {
		return ""
	}
	return p
}

var javaVersionPattern = regexp.MustCompile(`(?:openjdk|java|jre) (?:version ")?(?:1\.)?(\d+)`)

func ParseJavaVersion(output string) (int, error) {
	matches := javaVersionPattern.FindStringSubmatch(output)
	if len(matches) < 2 {
		return 0, fmt.Errorf("unable to parse Java version from: %s", strings.TrimSpace(output[:min(len(output), 100)]))
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, fmt.Errorf("invalid major version: %s", matches[1])
	}
	return major, nil
}

func CheckJavaVersion(javaPath string) (int, error) {
	cmd := exec.Command(javaPath, "-version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("java check failed: %w", err)
	}
	ver, err := ParseJavaVersion(string(out))
	if err != nil {
		log.L().Warn("could not parse java version", "path", javaPath, "output", string(out))
		return 0, err
	}
	return ver, nil
}

func SuggestJavaDownload() string {
	return "https://adoptium.net/temurin/releases/"
}
