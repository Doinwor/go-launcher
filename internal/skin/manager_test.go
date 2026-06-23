package skin

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveSkin_ValidDimensions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	if err := m.SaveSkin("test-uuid", data); err != nil {
		t.Fatalf("SaveSkin: %v", err)
	}

	if !m.HasSkin("test-uuid") {
		t.Error("expected skin to exist")
	}

	info, _ := os.Stat(m.SkinPath("test-uuid"))
	if info.Size() == 0 {
		t.Error("skin file should not be empty")
	}
}

func TestSaveSkin_64x32(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 32)
	if err := m.SaveSkin("uuid-32h", data); err != nil {
		t.Fatalf("SaveSkin 64x32: %v", err)
	}
}

func TestSaveSkin_InvalidWidth(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(128, 64)
	if err := m.SaveSkin("bad-width", data); err == nil {
		t.Error("expected error for 128px width")
	}
}

func TestSaveSkin_InvalidHeight(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 16)
	if err := m.SaveSkin("bad-height", data); err == nil {
		t.Error("expected error for 16px height")
	}
}

func TestSaveSkin_NotPNG(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	if err := m.SaveSkin("not-png", []byte("not an image")); err == nil {
		t.Error("expected error for non-PNG data")
	}
}

func TestSaveCape_AnyDimensions(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 32)
	if err := m.SaveCape("cape-uuid", data); err != nil {
		t.Fatalf("SaveCape: %v", err)
	}

	if !m.HasCape("cape-uuid") {
		t.Error("expected cape to exist")
	}
}

func TestSaveCape_InvalidPNG(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	if err := m.SaveCape("bad-cape", []byte{0, 1, 2}); err == nil {
		t.Error("expected error for invalid cape")
	}
}

func TestDeleteSkin(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 64)
	m.SaveSkin("del-uuid", data)

	if err := m.DeleteSkin("del-uuid"); err != nil {
		t.Fatalf("DeleteSkin: %v", err)
	}

	if m.HasSkin("del-uuid") {
		t.Error("skin should be deleted")
	}
}

func TestDeleteSkin_NotExists(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	if err := m.DeleteSkin("nonexistent"); err != nil {
		t.Errorf("deleting non-existent skin should not error: %v", err)
	}
}

func TestDeleteCape(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)
	m.EnsureDir()

	data := minimalPNG(64, 32)
	m.SaveCape("del-cape", data)

	if err := m.DeleteCape("del-cape"); err != nil {
		t.Fatalf("DeleteCape: %v", err)
	}

	if m.HasCape("del-cape") {
		t.Error("cape should be deleted")
	}
}

func TestSkinPath_Consistent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	p1 := m.SkinPath("550e8400-e29b-41d4-a716-446655440000")
	p2 := m.SkinPath("550e8400-e29b-41d4-a716-446655440000")
	if p1 != p2 {
		t.Error("skin path should be deterministic")
	}

	if !filepath.IsAbs(p1) {
		t.Errorf("expected absolute path, got %s", p1)
	}
}

func TestCapePath_Consistent(t *testing.T) {
	dir := t.TempDir()
	m := NewManager(dir)

	p := m.CapePath("some-uuid")
	if !strings.HasSuffix(p, "_cape.png") {
		t.Errorf("expected cape path to end with _cape.png, got %s", p)
	}
}

// minimalPNG generates a valid PNG of given dimensions.
func minimalPNG(w, h int) []byte {
	// Use Go's png encoder to create a minimal valid file via a temp approach
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	// fill with some pixels so it's not empty
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			idx := img.PixOffset(x, y)
			img.Pix[idx] = byte((x + y) & 0xff)     // R
			img.Pix[idx+1] = byte((x * y) & 0xff)    // G
			img.Pix[idx+2] = byte((x + y*3) & 0xff)  // B
			img.Pix[idx+3] = 255                      // A
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}
