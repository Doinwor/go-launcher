package skin

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
)

type Manager struct {
	skinsDir string
}

func NewManager(baseDir string) *Manager {
	return &Manager{
		skinsDir: filepath.Join(baseDir, "skins"),
	}
}

func (m *Manager) EnsureDir() error {
	return os.MkdirAll(m.skinsDir, 0755)
}

func (m *Manager) SaveSkin(uuid string, data []byte) error {
	if err := validatePNG(data, 64, []int{32, 64}); err != nil {
		return err
	}
	return os.WriteFile(m.SkinPath(uuid), data, 0644)
}

func (m *Manager) SaveCape(uuid string, data []byte) error {
	if err := validatePNG(data, 0, nil); err != nil {
		return err
	}
	return os.WriteFile(m.CapePath(uuid), data, 0644)
}

func (m *Manager) SkinPath(uuid string) string {
	return filepath.Join(m.skinsDir, strings.ToLower(uuid)+".png")
}

func (m *Manager) CapePath(uuid string) string {
	return filepath.Join(m.skinsDir, strings.ToLower(uuid)+"_cape.png")
}

func (m *Manager) HasSkin(uuid string) bool {
	_, err := os.Stat(m.SkinPath(uuid))
	return err == nil
}

func (m *Manager) HasCape(uuid string) bool {
	_, err := os.Stat(m.CapePath(uuid))
	return err == nil
}

func (m *Manager) DeleteSkin(uuid string) error {
	p := m.SkinPath(uuid)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(p)
}

func (m *Manager) DeleteCape(uuid string) error {
	p := m.CapePath(uuid)
	if _, err := os.Stat(p); os.IsNotExist(err) {
		return nil
	}
	return os.Remove(p)
}

func (m *Manager) SkinsDir() string {
	return m.skinsDir
}

func validatePNG(data []byte, expectMinWidth int, expectHeights []int) error {
	if len(data) < 8 || data[0] != 0x89 || string(data[1:4]) != "PNG" {
		return fmt.Errorf("not a valid PNG file")
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}

	if expectMinWidth > 0 && cfg.Width != expectMinWidth {
		return fmt.Errorf("expected width %dpx, got %dpx", expectMinWidth, cfg.Width)
	}

	if len(expectHeights) > 0 {
		valid := false
		for _, h := range expectHeights {
			if cfg.Height == h {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("expected height %v, got %dpx", expectHeights, cfg.Height)
		}
	}

	return nil
}
