package skin

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

type SkinTextures struct {
	Timestamp   int64       `json:"timestamp"`
	ProfileID   string      `json:"profileId"`
	ProfileName string      `json:"profileName"`
	Textures    TexturesMap `json:"textures"`
}

type TexturesMap struct {
	SKIN *TextureInfo `json:"SKIN,omitempty"`
	CAPE *TextureInfo `json:"CAPE,omitempty"`
}

type TextureInfo struct {
	URL      string            `json:"url"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (m *Manager) TexturePayload(uuid, username, baseURL string) (string, bool) {
	skinURL := baseURL + "/skin/" + strings.ToLower(uuid) + ".png"
	capeURL := baseURL + "/skin/" + strings.ToLower(uuid) + "_cape.png"

	t := SkinTextures{
		Timestamp:   time.Now().UnixMilli(),
		ProfileID:   strings.ReplaceAll(uuid, "-", ""),
		ProfileName: username,
	}

	hasSkin := m.HasSkin(uuid)
	hasCape := m.HasCape(uuid)

	if !hasSkin && !hasCape {
		return "", false
	}

	if hasSkin {
		t.Textures.SKIN = &TextureInfo{
			URL: skinURL,
		}
	}

	if hasCape {
		t.Textures.CAPE = &TextureInfo{
			URL: capeURL,
		}
	}

	data, _ := json.Marshal(t)
	return base64.StdEncoding.EncodeToString(data), true
}
