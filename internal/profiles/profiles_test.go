package profiles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultStore(t *testing.T) {
	s := DefaultStore()
	if s.data == nil {
		t.Fatal("data should not be nil")
	}
	if s.data.Profiles == nil {
		t.Error("profiles map should be initialized")
	}
	if s.data.AuthenticationDatabase == nil {
		t.Error("auth database should be initialized")
	}
	if s.data.ClientToken == "" {
		t.Error("clientToken should not be empty")
	}
}

func TestUpsertAndGetProfile(t *testing.T) {
	s := DefaultStore()
	p := Profile{
		ID:   "test-uuid",
		Name: "TestPlayer",
		Type: "offline",
	}

	s.UpsertProfile(p)

	got, ok := s.GetProfile("test-uuid")
	if !ok {
		t.Fatal("profile should exist")
	}
	if got.Name != "TestPlayer" {
		t.Errorf("expected TestPlayer, got %s", got.Name)
	}
}

func TestDeleteProfile(t *testing.T) {
	s := DefaultStore()
	s.UpsertProfile(Profile{ID: "del-uuid", Name: "ToDelete"})
	s.DeleteProfile("del-uuid")

	_, ok := s.GetProfile("del-uuid")
	if ok {
		t.Error("profile should have been deleted")
	}
}

func TestSetAuthAccount(t *testing.T) {
	s := DefaultStore()
	s.SetAuthAccount("uuid-1", "Player1", "token-1")

	acc, ok := s.data.AuthenticationDatabase["uuid-1"]
	if !ok {
		t.Fatal("auth account should exist")
	}
	if acc.Username != "Player1" {
		t.Errorf("expected Player1, got %s", acc.Username)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "launcher_profiles.json")

	s := DefaultStore()
	s.SetFilename(fn)
	s.UpsertProfile(Profile{ID: "save-test", Name: "SavePlayer"})

	if err := s.Save(); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	if _, err := os.Stat(fn); os.IsNotExist(err) {
		t.Error("file should exist after save")
	}

	s2 := DefaultStore()
	s2.SetFilename(fn)
	if err := s2.Load(); err != nil {
		t.Fatalf("load failed: %v", err)
	}

	p, ok := s2.GetProfile("save-test")
	if !ok {
		t.Fatal("profile should exist after load")
	}
	if p.Name != "SavePlayer" {
		t.Errorf("expected SavePlayer, got %s", p.Name)
	}
}
