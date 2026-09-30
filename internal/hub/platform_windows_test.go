package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSystemTrash(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "old notes.txt")
	folder := filepath.Join(dir, "Old folder")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(folder, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{file, folder} {
		if err := systemTrash(p); err != nil {
			t.Fatalf("systemTrash(%s): %v", p, err)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still there after moving it to the Recycle Bin", p)
		}
	}
}
