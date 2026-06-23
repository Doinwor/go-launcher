package skin

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestTexturePayload_NoSkin(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	payload, ok := m.TexturePayload("uuid-empty", "EmptyUser", "http://localhost:25566")
	if ok {
		t.Error("expected false when no skin or cape")
	}
	if payload != "" {
		t.Error("expected empty payload")
	}
}

func TestTexturePayload_WithSkin(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	if err := m.SaveSkin("uuid-skin", data); err != nil {
		t.Fatalf("SaveSkin: %v", err)
	}

	payload, ok := m.TexturePayload("uuid-skin", "SkinUser", "http://localhost:25566")
	if !ok {
		t.Fatal("expected true when skin exists")
	}
	if payload == "" {
		t.Fatal("expected non-empty payload")
	}

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}

	var textures SkinTextures
	if err := json.Unmarshal(decoded, &textures); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}

	if textures.ProfileName != "SkinUser" {
		t.Errorf("expected SkinUser, got %s", textures.ProfileName)
	}
	if textures.Textures.SKIN == nil {
		t.Fatal("expected SKIN texture")
	}
	if !strings.Contains(textures.Textures.SKIN.URL, "uuid-skin") {
		t.Errorf("expected URL containing uuid, got %s", textures.Textures.SKIN.URL)
	}
}

func TestTexturePayload_WithCape(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	if err := m.SaveSkin("uuid-sc", data); err != nil {
		t.Fatalf("SaveSkin: %v", err)
	}

	capeData := minimalPNG(64, 32)
	if err := m.SaveCape("uuid-sc", capeData); err != nil {
		t.Fatalf("SaveCape: %v", err)
	}

	payload, ok := m.TexturePayload("uuid-sc", "CapeUser", "http://localhost:25566")
	if !ok {
		t.Fatal("expected true when both exist")
	}

	decoded, _ := base64.StdEncoding.DecodeString(payload)
	var textures SkinTextures
	json.Unmarshal(decoded, &textures)

	if textures.Textures.SKIN == nil {
		t.Error("expected SKIN texture")
	}
	if textures.Textures.CAPE == nil {
		t.Error("expected CAPE texture")
	}
}

func TestTexturePayload_OnlyCape(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	capeData := minimalPNG(64, 32)
	if err := m.SaveCape("uuid-cape-only", capeData); err != nil {
		t.Fatalf("SaveCape: %v", err)
	}

	payload, ok := m.TexturePayload("uuid-cape-only", "CapeOnly", "http://localhost:25566")
	if !ok {
		t.Fatal("expected true when cape exists")
	}

	decoded, _ := base64.StdEncoding.DecodeString(payload)
	var textures SkinTextures
	json.Unmarshal(decoded, &textures)

	if textures.Textures.SKIN != nil {
		t.Error("expected no SKIN texture")
	}
	if textures.Textures.CAPE == nil {
		t.Error("expected CAPE texture")
	}
}

func TestTexturePayload_BaseURL(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	m.SaveSkin("url-check", data)

	payload, ok := m.TexturePayload("url-check", "URLUser", "http://192.168.1.100:25566")
	if !ok {
		t.Fatal("expected true")
	}

	decoded, _ := base64.StdEncoding.DecodeString(payload)
	var textures SkinTextures
	json.Unmarshal(decoded, &textures)

	if !strings.Contains(textures.Textures.SKIN.URL, "192.168.1.100:25566") {
		t.Errorf("expected base URL in texture, got %s", textures.Textures.SKIN.URL)
	}
}

func TestTexturePayload_Timestamp(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	m.SaveSkin("ts-check", data)

	payload, ok := m.TexturePayload("ts-check", "TSUser", "http://localhost:25566")
	if !ok {
		t.Fatal("expected true")
	}

	decoded, _ := base64.StdEncoding.DecodeString(payload)
	var textures SkinTextures
	json.Unmarshal(decoded, &textures)

	if textures.Timestamp <= 0 {
		t.Error("expected valid timestamp")
	}
}

func TestTexturePayload_ProfileIDNoDashes(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	m.SaveSkin("550e8400-e29b-41d4-a716-446655440000", data)

	payload, ok := m.TexturePayload("550e8400-e29b-41d4-a716-446655440000", "DashUser", "http://localhost:25566")
	if !ok {
		t.Fatal("expected true")
	}

	decoded, _ := base64.StdEncoding.DecodeString(payload)
	var textures SkinTextures
	json.Unmarshal(decoded, &textures)

	if strings.Contains(textures.ProfileID, "-") {
		t.Error("ProfileID should not contain dashes")
	}
	if textures.ProfileID != "550e8400e29b41d4a716446655440000" {
		t.Errorf("expected no-dash UUID, got %s", textures.ProfileID)
	}
}
