package auth

import (
	"strings"
	"testing"
)

func TestCreateOfflineAccount_Valid(t *testing.T) {
	acc, err := CreateOfflineAccount("Steve")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Username != "Steve" {
		t.Errorf("expected Steve, got %s", acc.Username)
	}
	if acc.UUID == "" {
		t.Fatal("UUID should not be empty")
	}
	if !strings.HasPrefix(acc.AccessToken, "offline-token:") {
		t.Errorf("accessToken should start with offline-token:, got %s", acc.AccessToken)
	}
	if acc.UserType != "mojang" {
		t.Errorf("expected userType mojang, got %s", acc.UserType)
	}
	if acc.XUID == "" {
		t.Error("XUID should not be empty")
	}
	if acc.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if acc.LastUsed != acc.CreatedAt {
		t.Error("LastUsed should equal CreatedAt for new accounts")
	}
}

func TestCreateOfflineAccount_EmptyNick(t *testing.T) {
	_, err := CreateOfflineAccount("")
	if err == nil {
		t.Error("expected error for empty nickname")
	}
}

func TestCreateOfflineAccount_WhitespaceNick(t *testing.T) {
	_, err := CreateOfflineAccount("   ")
	if err == nil {
		t.Error("expected error for whitespace-only nickname")
	}
}

func TestCreateOfflineAccount_TooLong(t *testing.T) {
	_, err := CreateOfflineAccount("abcdefghijklmnopq")
	if err == nil {
		t.Error("expected error for nickname > 16 chars")
	}
}

func TestCreateOfflineAccount_UUIDv3Consistent(t *testing.T) {
	a, _ := CreateOfflineAccount("Alice")
	b, _ := CreateOfflineAccount("Alice")
	if a.UUID != b.UUID {
		t.Errorf("same nickname should produce same UUID: %s vs %s", a.UUID, b.UUID)
	}
}

func TestCreateOfflineAccount_UUIDv3Different(t *testing.T) {
	a, _ := CreateOfflineAccount("Alice")
	b, _ := CreateOfflineAccount("Bob")
	if a.UUID == b.UUID {
		t.Error("different nicknames should produce different UUIDs")
	}
}

func TestCreateOfflineAccount_UUIDv3Format(t *testing.T) {
	acc, _ := CreateOfflineAccount("Steve")
	parts := strings.Split(acc.UUID, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 UUID parts, got %d", len(parts))
	}
	if acc.UUID[14] != '3' {
		t.Errorf("expected version digit 3, got %c", acc.UUID[14])
	}
}

func TestCreateOfflineAccount_TrimNickname(t *testing.T) {
	acc, err := CreateOfflineAccount("  Alex  ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if acc.Username != "Alex" {
		t.Errorf("expected trimmed Alex, got %q", acc.Username)
	}
}

func TestNewV3UUID_CaseInsensitive(t *testing.T) {
	a := newV3UUID("Steve")
	b := newV3UUID("steve")
	if a != b {
		t.Error("UUID v3 should be case-insensitive")
	}
}

func TestNewAccessToken(t *testing.T) {
	uid := "550e8400-e29b-41d4-a716-446655440000"
	token := newAccessToken(uid)
	expected := "offline-token:550e8400-e29b-41d4-a716-446655440000"
	if token != expected {
		t.Errorf("expected %q, got %q", expected, token)
	}
}

func TestNewXUID_Format(t *testing.T) {
	xuid := newXUID()
	if len(xuid) != 16 {
		t.Errorf("expected 16 hex chars, got %d: %s", len(xuid), xuid)
	}
	for _, c := range xuid {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("unexpected char %c in XUID", c)
		}
	}
}
