package launch

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type ProcessManager struct {
	mu       sync.RWMutex
	cmd      *exec.Cmd
	cancel   chan struct{}
	logs     []LogEntry
	logMu    sync.RWMutex
	listeners map[chan LogEntry]struct{}
	listenMu sync.RWMutex
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		cancel:    make(chan struct{}),
		listeners: make(map[chan LogEntry]struct{}),
	}
}

func (pm *ProcessManager) Start(javaPath string, args []string, workDir string) (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cmd != nil && pm.cmd.Process != nil && isProcessRunning(pm.cmd.Process.Pid) {
		return 0, fmt.Errorf("a game process is already running (PID %d)", pm.cmd.Process.Pid)
	}

	pm.cancel = make(chan struct{})

	pm.cmd = exec.Command(javaPath, args[1:]...)
	pm.cmd.Dir = workDir

	stdout, err := pm.cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := pm.cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := pm.cmd.Start(); err != nil {
		return 0, fmt.Errorf("start process: %w", err)
	}

	pid := pm.cmd.Process.Pid

	go pm.readPipe(stdout, "stdout")
	go pm.readPipe(stderr, "stderr")
	go pm.waitAndCleanup()

	return pid, nil
}

func (pm *ProcessManager) readPipe(pipe io.Reader, logType string) {
	reader := bufio.NewReader(pipe)
	for {
		select {
		case <-pm.cancel:
			return
		default:
		}

		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		entry := LogEntry{
			Line: strings.TrimRight(line, "\r\n"),
			Type: logType,
			Time: time.Now().UnixMilli(),
		}

		pm.logMu.Lock()
		pm.logs = append(pm.logs, entry)
		if len(pm.logs) > 10000 {
			pm.logs = pm.logs[len(pm.logs)-5000:]
		}
		pm.logMu.Unlock()

		pm.listenMu.RLock()
		for ch := range pm.listeners {
			select {
			case ch <- entry:
			default:
			}
		}
		pm.listenMu.RUnlock()
	}
}

func (pm *ProcessManager) waitAndCleanup() {
	pm.mu.RLock()
	cmd := pm.cmd
	pm.mu.RUnlock()

	if cmd == nil {
		return
	}

	cmd.Wait()

	pm.listenMu.RLock()
	for ch := range pm.listeners {
		close(ch)
	}
	pm.listenMu.RUnlock()
}

func (pm *ProcessManager) Stop() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.cmd == nil || pm.cmd.Process == nil {
		return fmt.Errorf("no running process")
	}

	pid := pm.cmd.Process.Pid

	if !isProcessRunning(pid) {
		pm.cmd = nil
		return nil
	}

	if err := killProcess(pid); err != nil {
		return fmt.Errorf("kill process %d: %w", pid, err)
	}

	close(pm.cancel)
	pm.cmd = nil
	return nil
}

func (pm *ProcessManager) Status() ProcessStatus {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if pm.cmd == nil || pm.cmd.Process == nil {
		return ProcessStatus{Running: false}
	}

	running := isProcessRunning(pm.cmd.Process.Pid)
	return ProcessStatus{
		Running: running,
		PID:     pm.cmd.Process.Pid,
	}
}

func (pm *ProcessManager) Subscribe() chan LogEntry {
	ch := make(chan LogEntry, 256)
	pm.listenMu.Lock()
	pm.listeners[ch] = struct{}{}
	pm.listenMu.Unlock()
	return ch
}

func (pm *ProcessManager) Unsubscribe(ch chan LogEntry) {
	pm.listenMu.Lock()
	delete(pm.listeners, ch)
	pm.listenMu.Unlock()
}

func (pm *ProcessManager) GetLogs(limit int) []LogEntry {
	pm.logMu.RLock()
	defer pm.logMu.RUnlock()
	if limit <= 0 || limit > len(pm.logs) {
		limit = len(pm.logs)
	}
	out := make([]LogEntry, limit)
	copy(out, pm.logs[len(pm.logs)-limit:])
	return out
}

func isProcessRunning(pid int) bool {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("tasklist", "/NH", "/FI", fmt.Sprintf("PID eq %d", pid))
		out, err := cmd.Output()
		if err != nil {
			return false
		}
		return strings.Contains(string(out), fmt.Sprintf("%d", pid))
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = process.Signal(os.Signal(nil))
	return err == nil
}

func killProcess(pid int) error {
	if runtime.GOOS == "windows" {
		cmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
		return cmd.Run()
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(os.Kill)
}

func BuildClasspath(mcDir string, libs []LibInfo) string {
	var parts []string
	for _, lib := range libs {
		if !rulesAllow(lib.Rules) {
			continue
		}
		path := ""
		if lib.Downloads != nil && lib.Downloads.Artifact != nil && lib.Downloads.Artifact.Path != "" {
			path = lib.Downloads.Artifact.Path
		} else if lib.Name != "" {
			path = LibPath(lib.Name)
		}
		if path != "" {
			parts = append(parts, filepath.Join(mcDir, "libraries", path))
		}
	}
	return strings.Join(parts, string(os.PathListSeparator))
}

var LauncherFeatures = map[string]bool{
	"is_demo_user":            false,
	"has_custom_resolution":   true,
	"has_quick_plays_support": false,
	"is_quick_play_singleplayer": false,
	"is_quick_play_multiplayer":  false,
	"is_quick_play_realms":       false,
}

func rulesAllow(rules []Rule) bool {
	if len(rules) == 0 {
		return true
	}
	allowed := false
	for _, r := range rules {
		if r.Features != nil {
			for k, v := range r.Features {
				if LauncherFeatures[k] != v {
					return r.Action != "allow"
				}
			}
			allowed = r.Action == "allow"
			continue
		}
		if r.OS == nil {
			allowed = r.Action == "allow"
			continue
		}
		if matchesOS(r.OS) {
			allowed = r.Action == "allow"
		}
	}
	return allowed
}

func matchesOS(rule *OSRule) bool {
	osName := strings.ToLower(runtime.GOOS)
	if rule.Name != "" && rule.Name != osName {
		return false
	}
	if rule.Arch != "" {
		arch := strings.ToLower(runtime.GOARCH)
		if rule.Arch != arch {
			return false
		}
	}
	return rule.Version == ""
}

func FilterLibsByOS(libs []LibInfo) []LibInfo {
	var out []LibInfo
	for _, lib := range libs {
		if rulesAllow(lib.Rules) {
			out = append(out, lib)
		}
	}
	return out
}

func extractArgString(v interface{}) string {
	switch s := v.(type) {
	case string:
		return s
	default:
		return ""
	}
}
