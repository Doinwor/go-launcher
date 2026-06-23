package authserver

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
)

type storedAccount struct {
	Username    string
	UUID        string
	AccessToken string
	ClientToken string
	Profiles    []ProfileResp
	JoinedServers map[string]bool
}

type Store struct {
	mu       sync.RWMutex
	accounts map[string]*storedAccount
	tokens   map[string]string
}

func NewStore() *Store {
	return &Store{
		accounts: make(map[string]*storedAccount),
		tokens:   make(map[string]string),
	}
}

func (s *Store) Register(username, uuid, clientToken string) *storedAccount {
	s.mu.Lock()
	defer s.mu.Unlock()

	accessToken := randomHex(16)

	acc := &storedAccount{
		Username:    username,
		UUID:        uuid,
		AccessToken: accessToken,
		ClientToken: clientToken,
		Profiles: []ProfileResp{{
			ID:   uuid,
			Name: username,
		}},
		JoinedServers: make(map[string]bool),
	}

	s.accounts[uuid] = acc
	s.tokens[accessToken] = uuid
	return acc
}

func (s *Store) Authenticate(username, clientToken string) (*storedAccount, string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, acc := range s.accounts {
		if acc.Username == username {
			newToken := randomHex(16)
			oldToken := acc.AccessToken

			if clientToken == "" {
				clientToken = randomHex(16)
			}

			acc.AccessToken = newToken
			acc.ClientToken = clientToken
			delete(s.tokens, oldToken)
			s.tokens[newToken] = acc.UUID

			return acc, clientToken
		}
	}

	return nil, ""
}

func (s *Store) TokenOwner(accessToken string) *storedAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uuid, ok := s.tokens[accessToken]
	if !ok {
		return nil
	}
	return s.accounts[uuid]
}

func (s *Store) Validate(accessToken, clientToken string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	uuid, ok := s.tokens[accessToken]
	if !ok {
		return false
	}

	acc := s.accounts[uuid]
	if acc == nil {
		return false
	}

	if clientToken != "" && acc.ClientToken != clientToken {
		return false
	}

	return true
}

func (s *Store) Refresh(accessToken, clientToken string) (*storedAccount, string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	uuid, ok := s.tokens[accessToken]
	if !ok {
		return nil, "", false
	}

	acc := s.accounts[uuid]
	if acc == nil {
		return nil, "", false
	}

	if clientToken != "" && acc.ClientToken != clientToken {
		return nil, "", false
	}

	newToken := randomHex(16)
	delete(s.tokens, accessToken)
	s.tokens[newToken] = uuid
	acc.AccessToken = newToken

	return acc, newToken, true
}

func (s *Store) Invalidate(accessToken, clientToken string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	uuid, ok := s.tokens[accessToken]
	if !ok {
		return false
	}

	acc := s.accounts[uuid]
	if acc == nil {
		return false
	}

	if clientToken != "" && acc.ClientToken != clientToken {
		return false
	}

	delete(s.tokens, accessToken)
	return true
}

func (s *Store) Signout(username string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for uuid, acc := range s.accounts {
		if acc.Username == username {
			delete(s.tokens, acc.AccessToken)
			delete(s.accounts, uuid)
			return true
		}
	}
	return false
}

func (s *Store) Join(accessToken, profileID, serverID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	uuid, ok := s.tokens[accessToken]
	if !ok {
		return false
	}

	acc := s.accounts[uuid]
	if acc == nil || acc.UUID != profileID {
		return false
	}

	acc.JoinedServers[serverID] = true
	return true
}

func (s *Store) HasJoined(username, serverID string) *storedAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, acc := range s.accounts {
		if acc.Username == username {
			if acc.JoinedServers[serverID] {
				return acc
			}
		}
	}
	return nil
}

func (s *Store) Profile(uuid string) *storedAccount {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accounts[uuid]
}

func randomHex(length int) string {
	b := make([]byte, length)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func init() {
	_ = make([]byte, 1)
}
