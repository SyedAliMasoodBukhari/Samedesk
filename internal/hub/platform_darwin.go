package hub

import (
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// hereTime is when the file's inode last changed on this Mac (Syncthing sets
// the sender's mtime, so this is when it actually arrived).
func hereTime(info fs.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Ctimespec.Sec, st.Ctimespec.Nsec)
	}
	return time.Time{}
}

func systemTrash(p string) error { return exec.Command("/usr/bin/trash", p).Run() }

func reveal(p string) { _ = exec.Command("open", "-R", p).Start() }

// OpenBrowser opens the dashboard in the default browser.
func OpenBrowser(url string) { _ = exec.Command("open", url).Start() }

// DisplayName is the computer's name as set in System Settings, e.g. "Ali's MacBook Air".
func DisplayName() string {
	if out, err := exec.Command("scutil", "--get", "ComputerName").Output(); err == nil {
		if n := strings.TrimSpace(string(out)); n != "" {
			return n
		}
	}
	h, _ := os.Hostname()
	return strings.TrimSuffix(h, ".local")
}
