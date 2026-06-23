package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const modrinthAPI = "https://api.modrinth.com/v2"

type ModrinthSearchHit struct {
	ProjectID     string   `json:"project_id"`
	ProjectType   string   `json:"project_type"`
	Slug          string   `json:"slug"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	IconURL       string   `json:"icon_url"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	LatestVersion string   `json:"latest_version"`
}

type ModrinthSearchResp struct {
	Hits   []ModrinthSearchHit `json:"hits"`
	Total  int                 `json:"total_hits"`
	Offset int                 `json:"offset"`
	Limit  int                 `json:"limit"`
}

type ModrinthVersion struct {
	ID           string   `json:"id"`
	ProjectID    string   `json:"project_id"`
	Name         string   `json:"name"`
	VersionNum   string   `json:"version_number"`
	GameVersions []string `json:"game_versions"`
	Loaders      []string `json:"loaders"`
	Files        []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Size     int64  `json:"size"`
	} `json:"files"`
}

func SearchModrinth(query, gameVersion string, loader LoaderType, limit, offset int) (*SearchResult, error) {
	params := url.Values{}
	params.Set("query", query)
	params.Set("limit", fmt.Sprintf("%d", limit))
	params.Set("offset", fmt.Sprintf("%d", offset))

	var facets []string
	facets = append(facets, fmt.Sprintf(`["project_type:mod"]`))
	if gameVersion != "" {
		facets = append(facets, fmt.Sprintf(`["versions:%s"]`, gameVersion))
	}
	if loader != "" && loader != LoaderVanilla {
		facets = append(facets, fmt.Sprintf(`["categories:%s"]`, string(loader)))
	}

	if len(facets) > 0 {
		params.Set("facets", "["+strings.Join(facets, ",")+"]")
	}

	searchURL := modrinthAPI + "/search?" + params.Encode()

	resp, err := http.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("modrinth search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("modrinth search HTTP %d: %s", resp.StatusCode, string(body))
	}

	var mr ModrinthSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("modrinth search parse: %w", err)
	}

	result := &SearchResult{
		Total:  mr.Total,
		Limit:  mr.Limit,
		Offset: mr.Offset,
	}

	for _, hit := range mr.Hits {
		loaderType := LoaderVanilla
		for _, l := range hit.Loaders {
			if l == "forge" {
				loaderType = LoaderForge
				break
			} else if l == "fabric" {
				loaderType = LoaderFabric
				break
			} else if l == "quilt" {
				loaderType = LoaderQuilt
				break
			}
		}

		gv := ""
		if len(hit.GameVersions) > 0 {
			gv = hit.GameVersions[0]
		}

		result.Hits = append(result.Hits, ModInfo{
			ID:          hit.ProjectID,
			Name:        hit.Title,
			Description: hit.Description,
			FileName:    hit.Slug + ".jar",
			Source:      SourceModrinth,
			ProjectID:   hit.ProjectID,
			GameVersion: gv,
			Loader:      loaderType,
			IconURL:     hit.IconURL,
			Enabled:     true,
		})
	}

	return result, nil
}

func FetchModrinthVersion(projectID, gameVersion string, loader LoaderType) (*ModrinthVersion, error) {
	url := fmt.Sprintf("%s/project/%s/version", modrinthAPI, projectID)
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("modrinth versions request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("modrinth versions HTTP %d", resp.StatusCode)
	}

	var versions []ModrinthVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("modrinth versions parse: %w", err)
	}

	if gameVersion == "" && loader == "" {
		if len(versions) > 0 {
			return &versions[0], nil
		}
		return nil, fmt.Errorf("no versions found for project %s", projectID)
	}

	for _, v := range versions {
		gvMatch := gameVersion == ""
		for _, gv := range v.GameVersions {
			if gv == gameVersion {
				gvMatch = true
				break
			}
		}
		if !gvMatch {
			continue
		}

		if loader != "" && loader != LoaderVanilla {
			loaderMatch := false
			for _, l := range v.Loaders {
				if l == string(loader) {
					loaderMatch = true
					break
				}
			}
			if !loaderMatch {
				continue
			}
		}

		return &v, nil
	}

	if len(versions) > 0 {
		return &versions[0], nil
	}

	return nil, fmt.Errorf("no matching version for %s %s %s", projectID, gameVersion, loader)
}
