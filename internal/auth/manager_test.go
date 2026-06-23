package auth

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManager_CreateAndGetActive(t *testing.T) {
	m := newTestManager(t)

	acc, err := m.CreateAccount("Steve")
	if err != nil {
		t.Fatalf("CreateAccount failed: %v", err)
	}

	active := m.GetActiveAccount()
	if active == nil {
		t.Fatal("active account should not be nil")
	}
	if active.UUID != acc.UUID {
		t.Errorf("expected active UUID %s, got %s", acc.UUID, active.UUID)
	}
}

func TestManager_ActiveUUID(t *testing.T) {
	m := newTestManager(t)
	if m.ActiveUUID() != "" {
		t.Error("expected empty active UUID for fresh manager")
	}

	m.CreateAccount("Player1")
	if m.ActiveUUID() == "" {
		t.Error("active UUID should be set after creating first account")
	}
}

func TestManager_SwitchAccount(t *testing.T) {
	m := newTestManager(t)

	a1, _ := m.CreateAccount("Alice")
	a2, _ := m.CreateAccount("Bob")

	if err := m.SwitchAccount(a2.UUID); err != nil {
		t.Fatalf("SwitchAccount failed: %v", err)
	}

	active := m.GetActiveAccount()
	if active.UUID != a2.UUID {
		t.Errorf("expected active %s, got %s", a2.UUID, active.UUID)
	}

	if err := m.SwitchAccount(a1.UUID); err != nil {
		t.Fatalf("SwitchAccount back failed: %v", err)
	}

	active = m.GetActiveAccount()
	if active.UUID != a1.UUID {
		t.Errorf("expected active %s, got %s", a1.UUID, active.UUID)
	}
}

func TestManager_SwitchAccount_NotFound(t *testing.T) {
	m := newTestManager(t)
	err := m.SwitchAccount("nonexistent-uuid")
	if err == nil {
		t.Error("expected error for nonexistent UUID")
	}
}

func TestManager_ListAccounts(t *testing.T) {
	m := newTestManager(t)

	m.CreateAccount("One")
	m.CreateAccount("Two")
	m.CreateAccount("Three")

	list := m.ListAccounts()
	if len(list) != 3 {
		t.Errorf("expected 3 accounts, got %d", len(list))
	}
}

func TestManager_GetAccount(t *testing.T) {
	m := newTestManager(t)
	acc, _ := m.CreateAccount("Finder")

	got := m.GetAccount(acc.UUID)
	if got == nil {
		t.Fatal("GetAccount returned nil")
	}
	if got.Username != "Finder" {
		t.Errorf("expected Finder, got %s", got.Username)
	}

	missing := m.GetAccount("no-such-uuid")
	if missing != nil {
		t.Error("expected nil for missing account")
	}
}

func TestManager_RemoveAccount(t *testing.T) {
	m := newTestManager(t)
	_, _ = m.CreateAccount("Keep")
	a2, _ := m.CreateAccount("Remove")

	if err := m.RemoveAccount(a2.UUID); err != nil {
		t.Fatalf("RemoveAccount failed: %v", err)
	}

	list := m.ListAccounts()
	if len(list) != 1 {
		t.Errorf("expected 1 account after removal, got %d", len(list))
	}

	if m.GetAccount(a2.UUID) != nil {
		t.Error("removed account should be nil")
	}
}

func TestManager_RemoveAccount_NotFound(t *testing.T) {
	m := newTestManager(t)
	err := m.RemoveAccount("no-such-uuid")
	if err == nil {
		t.Error("expected error for removing nonexistent account")
	}
}

func TestManager_RemoveActiveSwitchesToFirst(t *testing.T) {
	m := newTestManager(t)
	a1, _ := m.CreateAccount("First")
	a2, _ := m.CreateAccount("Second")

	m.SwitchAccount(a1.UUID)
	m.RemoveAccount(a1.UUID)

	active := m.GetActiveAccount()
	if active == nil || active.UUID != a2.UUID {
		t.Errorf("expected active to switch to %s, got %v", a2.UUID, active)
	}
}

func TestManager_Persistence(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "accounts.json")

	m1 := NewManager(fn)
	m1.CreateAccount("Persist1")
	m1.CreateAccount("Persist2")
	m1.SwitchAccount(m1.ListAccounts()[1].UUID)

	m2 := NewManager(fn)
	list := m2.ListAccounts()
	if len(list) != 2 {
		t.Errorf("expected 2 accounts after reload, got %d", len(list))
	}

	active := m2.GetActiveAccount()
	if active == nil {
		t.Fatal("active account should survive reload")
	}

	names := map[string]bool{}
	for _, a := range list {
		names[a.Username] = true
	}
	if !names["Persist1"] || !names["Persist2"] {
		t.Errorf("both usernames should be present after reload: %v", names)
	}
}

func TestManager_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	fn := filepath.Join(dir, "accounts.json")
	os.WriteFile(fn, []byte("{}"), 0644)

	m := NewManager(fn)
	if m.GetActiveAccount() != nil {
		t.Error("active account should be nil for empty file")
	}
}

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	return NewManager(filepath.Join(dir, "accounts.json"))
}
