package auth

import (
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

var dnsNamespace = [16]byte{0x6b, 0xa7, 0xb8, 0x10, 0x9d, 0xad, 0x11, 0xd1, 0x80, 0xb4, 0x00, 0xc0, 0x4f, 0xd4, 0x30, 0xc8}

type Account struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	AccessToken string `json:"accessToken"`
	UserType    string `json:"userType"`
	XUID        string `json:"xuid"`
	CreatedAt   string `json:"createdAt"`
	LastUsed    string `json:"lastUsed"`
}

func newV3UUID(name string) string {
	h := md5.New()
	h.Write(dnsNamespace[:])
	h.Write([]byte(strings.ToLower(name)))
	hash := h.Sum(nil)
	hash[6] = (hash[6] & 0x0f) | 0x30
	hash[8] = (hash[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		hash[0:4], hash[4:6], hash[6:8], hash[8:10], hash[10:16])
}

func newAccessToken(uuid string) string {
	return "offline-token:" + uuid
}

func newXUID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func CreateOfflineAccount(nickname string) (*Account, error) {
	nickname = strings.TrimSpace(nickname)
	if nickname == "" {
		return nil, fmt.Errorf("nickname cannot be empty")
	}
	if len(nickname) > 16 {
		return nil, fmt.Errorf("nickname too long (max 16 characters)")
	}

	uid := newV3UUID(nickname)
	now := time.Now().UTC().Format(time.RFC3339)

	acc := &Account{
		UUID:        uid,
		Username:    nickname,
		AccessToken: newAccessToken(uid),
		UserType:    "mojang",
		XUID:        newXUID(),
		CreatedAt:   now,
		LastUsed:    now,
	}

	return acc, nil
}
