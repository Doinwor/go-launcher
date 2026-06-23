package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type accountFile struct {
	Accounts   map[string]*Account `json:"accounts"`
	ActiveUUID string              `json:"activeUUID"`
}

type Manager struct {
	mu       sync.RWMutex
	accounts map[string]*Account
	active   string
	filePath string
}

func NewManager(filePath string) *Manager {
	m := &Manager{
		accounts: make(map[string]*Account),
		filePath: filePath,
	}
	m.load()
	return m
}

func (m *Manager) CreateAccount(nickname string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc, err := CreateOfflineAccount(nickname)
	if err != nil {
		return nil, err
	}

	m.accounts[acc.UUID] = acc

	if m.active == "" {
		m.active = acc.UUID
	}

	if err := m.save(); err != nil {
		return nil, fmt.Errorf("failed to persist accounts: %w", err)
	}

	return acc, nil
}

func (m *Manager) SwitchAccount(uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.accounts[uuid]; !exists {
		return fmt.Errorf("account %s not found", uuid)
	}

	m.active = uuid
	m.accounts[uuid].LastUsed = time.Now().UTC().Format(time.RFC3339)

	return m.save()
}

func (m *Manager) GetActiveAccount() *Account {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.active == "" {
		return nil
	}
	return m.accounts[m.active]
}

func (m *Manager) ListAccounts() []*Account {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Account, 0, len(m.accounts))
	for _, acc := range m.accounts {
		result = append(result, acc)
	}
	return result
}

func (m *Manager) GetAccount(uuid string) *Account {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.accounts[uuid]
}

func (m *Manager) ClearActive() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.active = ""
	return m.save()
}

func (m *Manager) RenameAccount(uuid, newNickname string) (*Account, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	acc, exists := m.accounts[uuid]
	if !exists {
		return nil, fmt.Errorf("account not found")
	}

	newNickname = strings.TrimSpace(newNickname)
	if newNickname == "" {
		return nil, fmt.Errorf("nickname cannot be empty")
	}
	if len(newNickname) > 16 {
		return nil, fmt.Errorf("nickname too long (max 16 characters)")
	}

	acc.Username = newNickname
	acc.LastUsed = time.Now().UTC().Format(time.RFC3339)

	if err := m.save(); err != nil {
		return nil, fmt.Errorf("failed to persist accounts: %w", err)
	}

	return acc, nil
}

func (m *Manager) RemoveAccount(uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.accounts[uuid]; !exists {
		return fmt.Errorf("account %s not found", uuid)
	}

	delete(m.accounts, uuid)

	if m.active == uuid {
		m.active = ""
		for k := range m.accounts {
			m.active = k
			break
		}
	}

	return m.save()
}

func (m *Manager) ActiveUUID() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *Manager) load() error {
	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var file accountFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	if file.Accounts != nil {
		m.accounts = file.Accounts
	}
	m.active = file.ActiveUUID
	return nil
}

func (m *Manager) save() error {
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	file := accountFile{
		Accounts:   m.accounts,
		ActiveUUID: m.active,
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.filePath, data, 0644)
}
