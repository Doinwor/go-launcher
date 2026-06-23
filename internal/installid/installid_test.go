package installid

import (
	"testing"
)

func TestNewAndGet(t *testing.T) {
	dir := t.TempDir()
	id1 := New(dir)
	if id1.Get() == "" {
		t.Fatal("expected non-empty ID")
	}

	id2 := New(dir)
	if id2.Get() != id1.Get() {
		t.Error("expected same ID for same dir")
	}
}

func TestID_Persistence(t *testing.T) {
	dir := t.TempDir()
	id1 := New(dir)
	first := id1.Get()

	id2 := New(dir)
	second := id2.Get()

	if first != second {
		t.Errorf("expected persistent ID: %s vs %s", first, second)
	}
}

func TestID_Unique(t *testing.T) {
	id1 := New(t.TempDir())
	id2 := New(t.TempDir())

	if id1.Get() == id2.Get() {
		t.Error("expected unique IDs for different dirs")
	}
}

func TestID_Length(t *testing.T) {
	id := New(t.TempDir())
	if len(id.Get()) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(id.Get()))
	}
}

func TestID_String(t *testing.T) {
	id := New(t.TempDir())
	if id.String() != id.Get() {
		t.Error("String() should return same as Get()")
	}
}

func TestID_Verify(t *testing.T) {
	dir := t.TempDir()
	id := New(dir)
	val := id.Get()

	if !id.Verify(val) {
		t.Error("Verify should return true for correct ID")
	}
	if id.Verify("wrong") {
		t.Error("Verify should return false for wrong ID")
	}
}
