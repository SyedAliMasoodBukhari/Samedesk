package hub

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHiddenName(t *testing.T) {
	for name, want := range map[string]bool{
		".samedesk": true, ".stfolder": true, ".DS_Store": true, ".hidden": true,
		"desktop.ini": true, "Desktop.ini": true, "Thumbs.db": true, "$RECYCLE.BIN": true,
		"Notes": false, "report.pdf": false, "~$report.docx": false, "dot.in.name": false,
	} {
		if got := hiddenName(name); got != want {
			t.Errorf("hiddenName(%q) = %v; want %v", name, got, want)
		}
	}
}

// folder makes a shared folder with things every OS hides one way or another.
func folder(t *testing.T) (*Hub, string) {
	root := t.TempDir()
	for _, p := range []string{".samedesk/clips/x.json", "desktop.ini", "Photos/Thumbs.db", "Photos/beach.jpg", ".notes/today.md", "Visible/a.txt"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Hub{root: root}, root
}

func TestHideAll(t *testing.T) {
	h, root := folder(t)
	h.hideAll()
	for _, p := range []string{".samedesk", "desktop.ini", "Photos/Thumbs.db", ".notes"} {
		if !isHidden(t, filepath.Join(root, filepath.FromSlash(p))) {
			t.Errorf("%s should be hidden", p)
		}
	}
	for _, p := range []string{"Photos", "Photos/beach.jpg", "Visible", "Visible/a.txt"} {
		if isHidden(t, filepath.Join(root, filepath.FromSlash(p))) {
			t.Errorf("%s should stay visible", p)
		}
	}
}

func TestHidePaths(t *testing.T) {
	h, root := folder(t)
	h.hidePaths([]string{"Photos/Thumbs.db", ".notes/today.md", "Visible/a.txt", "gone/.missing"})
	for _, p := range []string{"Photos/Thumbs.db", ".notes"} {
		if !isHidden(t, filepath.Join(root, filepath.FromSlash(p))) {
			t.Errorf("%s should be hidden", p)
		}
	}
	if isHidden(t, filepath.Join(root, "Visible", "a.txt")) {
		t.Error("Visible/a.txt should stay visible")
	}
}
