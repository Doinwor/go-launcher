package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const curseforgeAPI = "https://api.curseforge.com/v1"

type CurseForgeClient struct {
	apiKey string
	client *http.Client
}

func NewCurseForgeClient(apiKey string) *CurseForgeClient {
	return &CurseForgeClient{
		apiKey: apiKey,
		client: &http.Client{},
	}
}

type CurseForgeSearchHit struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Summary     string `json:"summary"`
	Logo        *struct {
		URL string `json:"url"`
	} `json:"logo"`
	LatestFiles []struct {
		ID            int      `json:"id"`
		FileName      string   `json:"fileName"`
		FileSize      int64    `json:"fileSize"`
		DownloadURL   string   `json:"downloadUrl"`
		GameVersions  []string `json:"gameVersions"`
	} `json:"latestFiles"`
}

type CurseForgeSearchResp struct {
	Data []CurseForgeSearchHit `json:"data"`
}

func (c *CurseForgeClient) Search(query, gameVersion string, loader LoaderType, limit, offset int) (*SearchResult, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("CurseForge API key not configured")
	}

	params := url.Values{}
	params.Set("gameId", "432")
	params.Set("searchFilter", query)
	params.Set("pageSize", fmt.Sprintf("%d", limit))
	params.Set("index", fmt.Sprintf("%d", offset))

	if gameVersion != "" {
		params.Set("gameVersion", gameVersion)
	}

	if loader != "" && loader != LoaderVanilla {
		classID := "6"
		switch loader {
		case LoaderForge:
			classID = "6"
		case LoaderFabric:
			classID = "634"
		case LoaderQuilt:
			classID = "676" // TODO: verify this class ID for Quilt
		}
		params.Set("classId", classID)
	}

	searchURL := curseforgeAPI + "/mods/search?" + params.Encode()

	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("curseforge search: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("curseforge search HTTP %d: %s", resp.StatusCode, string(body))
	}

	var cf CurseForgeSearchResp
	if err := json.NewDecoder(resp.Body).Decode(&cf); err != nil {
		return nil, fmt.Errorf("curseforge search parse: %w", err)
	}

	result := &SearchResult{
		Total:  len(cf.Data),
		Limit:  limit,
		Offset: offset,
		Hits:   make([]ModInfo, 0),
	}

	for _, hit := range cf.Data {
		fileName := hit.Slug + ".jar"
		downloadURL := ""
		gv := gameVersion

		if len(hit.LatestFiles) > 0 {
			fileName = hit.LatestFiles[0].FileName
			downloadURL = hit.LatestFiles[0].DownloadURL
			if gv == "" && len(hit.LatestFiles[0].GameVersions) > 0 {
				gv = hit.LatestFiles[0].GameVersions[0]
			}
		}

		iconURL := ""
		if hit.Logo != nil {
			iconURL = hit.Logo.URL
		}

		result.Hits = append(result.Hits, ModInfo{
			ID:          fmt.Sprintf("%d", hit.ID),
			Name:        hit.Name,
			Description: hit.Summary,
			FileName:    fileName,
			Source:      SourceCurseForge,
			ProjectID:   fmt.Sprintf("%d", hit.ID),
			GameVersion: gv,
			Loader:      loader,
			IconURL:     iconURL,
			URL:         downloadURL,
			Enabled:     true,
		})
	}

	return result, nil
}

func (c *CurseForgeClient) ModURL(modID string) (string, error) {
	if c.apiKey == "" {
		return "", fmt.Errorf("CurseForge API key not configured")
	}

	u := fmt.Sprintf("%s/mods/%s/files?pageSize=1", curseforgeAPI, modID)
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("curseforge files: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("curseforge files HTTP %d", resp.StatusCode)
	}

	var data struct {
		Data []struct {
			DownloadURL string `json:"downloadUrl"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return "", err
	}

	if len(data.Data) > 0 && data.Data[0].DownloadURL != "" {
		return data.Data[0].DownloadURL, nil
	}

	return "", fmt.Errorf("no download URL found for mod %s", modID)
}

func IsValidCurseForgeKey(apiKey string) bool {
	req, err := http.NewRequest("GET", curseforgeAPI+"/games/432", nil)
	if err != nil {
		return false
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// ParseCurseForgeURL extracts mod ID from a CurseForge URL like https://www.curseforge.com/minecraft/mc-mods/jei
func ParseCurseForgeURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i, part := range parts {
		if part == "mc-mods" && i+1 < len(parts) {
			slug := parts[i+1]
			return slug, nil
		}
	}

	return "", fmt.Errorf("could not parse CurseForge URL: %s", rawURL)
}
