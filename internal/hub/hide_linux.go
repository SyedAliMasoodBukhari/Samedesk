package hub

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// hideNative lists files that other computers hide, but whose names don't start
// with a dot, in their folder's .hidden file, which GNOME Files, KDE Dolphin and
// most other file managers respect.
func hideNative(paths []string) {
	byDir := map[string][]string{}
	for _, p := range paths {
		if name := filepath.Base(p); !strings.HasPrefix(name, ".") {
			byDir[filepath.Dir(p)] = append(byDir[filepath.Dir(p)], name)
		}
	}
	for dir, names := range byDir {
		list := filepath.Join(dir, ".hidden")
		have := map[string]bool{}
		old, _ := os.ReadFile(list)
		for _, l := range strings.Split(string(old), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				have[l] = true
			}
		}
		changed := false
		for _, n := range names {
			if !have[n] {
				have[n], changed = true, true
			}
		}
		if !changed {
			continue
		}
		var lines []string
		for n := range have {
			lines = append(lines, n)
		}
		sort.Strings(lines)
		_ = os.WriteFile(list, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
}
