package download

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/offline-launcher/internal/download/types"
)

var manifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest_v2.json"

func fetchManifest(client *httpClient) (*types.VersionManifest, error) {
	data, err := client.getBytes(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}

	var manifest types.VersionManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}

	return &manifest, nil
}

func fetchVersion(client *httpClient, url string) (*types.VersionInfo, error) {
	data, err := client.getBytes(url)
	if err != nil {
		return nil, fmt.Errorf("fetch version: %w", err)
	}

	var version types.VersionInfo
	if err := json.Unmarshal(data, &version); err != nil {
		return nil, fmt.Errorf("parse version: %w", err)
	}

	return &version, nil
}

func parseAssetIndex(path string) ([]types.AssetInfo, error) {
	data, err := readFileBytes(path)
	if err != nil {
		return nil, fmt.Errorf("read asset index: %w", err)
	}

	var index types.AssetIndexData
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("parse asset index: %w", err)
	}

	var assets []types.AssetInfo
	for p, obj := range index.Objects {
		assets = append(assets, types.AssetInfo{
			Hash: obj.Hash,
			Size: obj.Size,
			Path: p,
		})
	}

	return assets, nil
}

func readFileBytes(path string) ([]byte, error) {
	return os.ReadFile(path)
}
