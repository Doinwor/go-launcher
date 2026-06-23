package uuid

import (
	"strings"
	"testing"
)

func TestNewV3FromName_Format(t *testing.T) {
	uid := NewV3FromName("TestPlayer")
	parts := strings.Split(uid, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 parts, got %d", len(parts))
	}
	if len(uid) != 36 {
		t.Errorf("expected 36 chars, got %d", len(uid))
	}
}

func TestNewV3FromName_Version(t *testing.T) {
	uid := NewV3FromName("Steve")
	versionChar := uid[14]
	if versionChar != '3' {
		t.Errorf("expected version 3, got %c", versionChar)
	}
}

func TestNewV3FromName_Consistent(t *testing.T) {
	a := NewV3FromName("Alice")
	b := NewV3FromName("Alice")
	if a != b {
		t.Errorf("same name should produce same UUID: %s vs %s", a, b)
	}
}

func TestNewV3FromName_Different(t *testing.T) {
	a := NewV3FromName("Alice")
	b := NewV3FromName("Bob")
	if a == b {
		t.Error("different names should produce different UUIDs")
	}
}

func TestNewV3FromName_CaseInsensitive(t *testing.T) {
	a := NewV3FromName("Steve")
	b := NewV3FromName("steve")
	if a != b {
		t.Error("UUID should be case-insensitive")
	}
}

func TestNewV4_Format(t *testing.T) {
	uid := NewV4()
	parts := strings.Split(uid, "-")
	if len(parts) != 5 {
		t.Errorf("expected 5 parts, got %d", len(parts))
	}
	if len(uid) != 36 {
		t.Errorf("expected 36 chars, got %d", len(uid))
	}
}

func TestNewV4_Version(t *testing.T) {
	uid := NewV4()
	versionChar := uid[14]
	if versionChar != '4' {
		t.Errorf("expected version 4, got %c", versionChar)
	}
}

func TestNewV4_Unique(t *testing.T) {
	a := NewV4()
	b := NewV4()
	if a == b {
		t.Error("V4 UUIDs should be unique")
	}
}
