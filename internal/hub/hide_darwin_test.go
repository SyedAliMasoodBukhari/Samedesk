package hub

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"golang.org/x/sys/unix"
)

// isHidden is what Finder sees: a leading dot, or the hidden flag.
func isHidden(t *testing.T, p string) bool {
	info, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	return strings.HasPrefix(filepath.Base(p), ".") || info.Sys().(*syscall.Stat_t).Flags&unix.UF_HIDDEN != 0
}
