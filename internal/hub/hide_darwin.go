package hub

import (
	"os"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// hideNative sets Finder's hidden flag on files that other computers hide but
// whose names don't start with a dot (Finder already hides those).
func hideNative(paths []string) {
	for _, p := range paths {
		info, err := os.Lstat(p)
		if err != nil || strings.HasPrefix(info.Name(), ".") {
			continue
		}
		if st, ok := info.Sys().(*syscall.Stat_t); ok && st.Flags&unix.UF_HIDDEN == 0 {
			_ = unix.Chflags(p, int(st.Flags|unix.UF_HIDDEN))
		}
	}
}
