package mods

import (
	"fmt"
	"path/filepath"
)

type ModSource string

const (
	SourceModrinth  ModSource = "modrinth"
	SourceCurseForge ModSource = "curseforge"
	SourceDirect    ModSource = "direct"
)

type LoaderType string

const (
	LoaderForge LoaderType = "forge"
	LoaderFabric LoaderType = "fabric"
	LoaderQuilt  LoaderType = "quilt"
	LoaderVanilla LoaderType = "vanilla"
)

type ModInfo struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version,omitempty"`
	Description string   `json:"description,omitempty"`
	FileName    string   `json:"fileName"`
	FileSize    int64    `json:"fileSize,omitempty"`
	Enabled     bool     `json:"enabled"`
	Source      ModSource `json:"source,omitempty"`
	ProjectID   string   `json:"projectId,omitempty"`
	GameVersion string   `json:"gameVersion,omitempty"`
	Loader      LoaderType `json:"loader,omitempty"`
	URL         string   `json:"url,omitempty"`
	IconURL     string   `json:"iconUrl,omitempty"`
	Side        string   `json:"side,omitempty"`
}

type LoaderVersion struct {
	Loader  LoaderType `json:"loader"`
	Version string     `json:"version"`
	Stable  bool       `json:"stable"`
}

type SearchResult struct {
	Hits  []ModInfo `json:"hits"`
	Total int       `json:"total"`
	Limit int       `json:"limit"`
	Offset int      `json:"offset"`
}

type InstallModReq struct {
	Source      ModSource  `json:"source"`
	ProjectID   string     `json:"projectId"`
	VersionID   string     `json:"versionId,omitempty"`
	URL         string     `json:"url,omitempty"`
	FileName    string     `json:"fileName,omitempty"`
	GameVersion string     `json:"gameVersion,omitempty"`
	Loader      LoaderType `json:"loader,omitempty"`
}

type InstallLoaderReq struct {
	Loader      LoaderType `json:"loader"`
	MCVersion   string     `json:"mcVersion"`
	LoaderVersion string   `json:"loaderVersion,omitempty"`
}

type InstallLoaderResp struct {
	Loader        LoaderType `json:"loader"`
	MCVersion     string     `json:"mcVersion"`
	LoaderVersion string     `json:"loaderVersion"`
	ProfileID     string     `json:"profileId"`
	JavaPath      string     `json:"javaPath,omitempty"`
}

func (m ModInfo) DisabledName() string {
	return m.FileName + ".disabled"
}

func ProfileIDForLoader(loader LoaderType, mcVersion, loaderVersion string) string {
	switch loader {
	case LoaderForge:
		return "forge-" + mcVersion + "-" + loaderVersion
	case LoaderFabric:
		return "fabric-loader-" + loaderVersion + "-" + mcVersion
	case LoaderQuilt:
		return "quilt-loader-" + loaderVersion + "-" + mcVersion
	default:
		return mcVersion
	}
}

func ProfileDirName(profileID string) string {
	return profileID
}

func ModsDirForProfile(mcDir, profileID string) string {
	return filepath.Join(mcDir, "mods")
}

func LoaderDisplayName(loader LoaderType) string {
	switch loader {
	case LoaderForge:
		return "Forge"
	case LoaderFabric:
		return "Fabric"
	case LoaderQuilt:
		return "Quilt"
	default:
		return "Vanilla"
	}
}

var ErrLoaderNotFound = fmt.Errorf("no compatible loader version found")
