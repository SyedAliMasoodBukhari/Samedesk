package hub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isHidden is what Linux file managers see: a leading dot, or a line in .hidden.
func isHidden(t *testing.T, p string) bool {
	name := filepath.Base(p)
	if strings.HasPrefix(name, ".") {
		return true
	}
	list, _ := os.ReadFile(filepath.Join(filepath.Dir(p), ".hidden"))
	for _, l := range strings.Split(string(list), "\n") {
		if l == name {
			return true
		}
	}
	return false
}
