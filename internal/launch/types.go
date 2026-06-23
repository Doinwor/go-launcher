package launch

type JavaVersionInfo struct {
	Component    string  `json:"component"`
	MajorVersion float64 `json:"majorVersion"`
}

	type VersionJSON struct {
	ID                string            `json:"id"`
	InheritsFrom      string            `json:"inheritsFrom"`
	Type              string            `json:"type"`
	MainClass         string            `json:"mainClass"`
	MinecraftArgs     string            `json:"minecraftArguments"`
	Arguments         *ArgsBlock        `json:"arguments"`
	Assets            string            `json:"assets"`
	AssetIndex        *AssetIndexRef    `json:"assetIndex"`
	Downloads         *VersionDownloads `json:"downloads"`
	Libraries         []LibInfo         `json:"libraries"`
	MinimumLauncherVersion int          `json:"minimumLauncherVersion"`
	JavaVersion       *JavaVersionInfo  `json:"javaVersion"`
	ReleaseTime       string            `json:"releaseTime"`
	Time              string            `json:"time"`
}

type ArgsBlock struct {
	Game []interface{} `json:"game"`
	JVM  []interface{} `json:"jvm"`
}

type AssetIndexRef struct {
	ID   string `json:"id"`
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

type VersionDownloads struct {
	Client *FileInfo `json:"client"`
	Server *FileInfo `json:"server"`
}

type FileInfo struct {
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
	Path string `json:"path"`
}

type LibInfo struct {
	Name      string           `json:"name"`
	URL       string           `json:"url"`
	Downloads *LibDownloads    `json:"downloads"`
	Natives   map[string]string `json:"natives"`
	Rules     []Rule           `json:"rules"`
}

type LibDownloads struct {
	Artifact    *FileInfo            `json:"artifact"`
	Classifiers map[string]*FileInfo `json:"classifiers"`
}

type Rule struct {
	Action   string           `json:"action"`
	OS       *OSRule          `json:"os"`
	Features map[string]bool  `json:"features"`
}

type OSRule struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

type LaunchConfig struct {
	MinecraftDir string   `json:"minecraftDir"`
	JavaPath     string   `json:"javaPath"`
	JVMArgs      []string `json:"jvmArgs"`
	MinMemory    int      `json:"minMemory"`
	MaxMemory    int      `json:"maxMemory"`
	WindowWidth  int      `json:"windowWidth"`
	WindowHeight int      `json:"windowHeight"`
}

type LogEntry struct {
	Line  string `json:"line"`
	Type  string `json:"type"` // "stdout" or "stderr"
	Time  int64  `json:"time"`
}

type ProcessStatus struct {
	Running bool   `json:"running"`
	PID     int    `json:"pid"`
	Version string `json:"version,omitempty"`
	Account string `json:"account,omitempty"`
}
