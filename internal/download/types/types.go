package types

type ProgressInfo struct {
	Task      string  `json:"task"`
	Completed int     `json:"completed"`
	Total     int     `json:"total"`
	Percent   float64 `json:"percent"`
}

type VersionManifest struct {
	Latest   LatestVersions `json:"latest"`
	Versions []VersionEntry `json:"versions"`
}

type LatestVersions struct {
	Release  string `json:"release"`
	Snapshot string `json:"snapshot"`
}

type VersionEntry struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Time        string `json:"time"`
	ReleaseTime string `json:"releaseTime"`
}

type VersionInfo struct {
	ID              string          `json:"id"`
	Type            string          `json:"type"`
	Time            string          `json:"time"`
	ReleaseTime     string          `json:"releaseTime"`
	SHA1            string          `json:"sha1"`
	Assets          string          `json:"assets"`
	AssetIndex      *AssetIndexRef  `json:"assetIndex"`
	Downloads       *VersionDownloads `json:"downloads"`
	Libraries       []LibraryInfo   `json:"libraries"`
	MainClass       string          `json:"mainClass"`
	MinecraftArgs   string          `json:"minecraftArguments"`
	URL             string          `json:"-"`
}

type AssetIndexRef struct {
	ID    string `json:"id"`
	SHA1  string `json:"sha1"`
	Size  int64  `json:"size"`
	URL   string `json:"url"`
	Total int64  `json:"total"`
}

type VersionDownloads struct {
	Client *FileInfo `json:"client"`
	Server *FileInfo `json:"server"`
}

type FileInfo struct {
	SHA1 string `json:"sha1"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
	Path string `json:"path,omitempty"`
}

type LibraryInfo struct {
	Name       string                  `json:"name"`
	Downloads  *LibraryDownloads       `json:"downloads"`
	Natives    map[string]string       `json:"natives"`
	Extract    *ExtractInfo            `json:"extract"`
	Rules      []Rule                  `json:"rules"`
}

type LibraryDownloads struct {
	Artifact    *FileInfo            `json:"artifact"`
	Classifiers map[string]*FileInfo `json:"classifiers"`
}

type ExtractInfo struct {
	Exclude []string `json:"exclude"`
}

type Rule struct {
	Action string `json:"action"`
	OS     *OSRule `json:"os"`
}

type OSRule struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Arch    string `json:"arch"`
}

type AssetInfo struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

type AssetIndexData struct {
	Objects map[string]AssetObject `json:"objects"`
}

type AssetObject struct {
	Hash string `json:"hash"`
	Size int64  `json:"size"`
}
