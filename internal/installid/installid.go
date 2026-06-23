package installid

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type ID struct {
	mu       sync.RWMutex
	id       string
	filePath string
}

func New(appDir string) *ID {
	return &ID{
		filePath: filepath.Join(appDir, ".install_id"),
	}
}

func (i *ID) Get() string {
	i.mu.RLock()
	if i.id != "" {
		defer i.mu.RUnlock()
		return i.id
	}
	i.mu.RUnlock()

	i.mu.Lock()
	defer i.mu.Unlock()

	if i.id != "" {
		return i.id
	}

	data, err := os.ReadFile(i.filePath)
	if err == nil && len(data) == 64 {
		i.id = string(data)
		return i.id
	}

	b := make([]byte, 32)
	rand.Read(b)
	i.id = hex.EncodeToString(b)

	os.MkdirAll(filepath.Dir(i.filePath), 0755)
	os.WriteFile(i.filePath, []byte(i.id), 0644)

	return i.id
}

func (i *ID) String() string {
	return i.Get()
}

func (i *ID) Verify(updateID string) bool {
	return updateID == i.Get()
}

func (i *ID) SignPayload(payload []byte) string {
	return fmt.Sprintf("%s-%x", i.Get(), payload)
}
