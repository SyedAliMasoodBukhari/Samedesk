package hub

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

// hereTime is when the file's inode last changed on this machine (Syncthing
// sets the sender's mtime, so this is when it actually arrived).
func hereTime(info fs.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Ctim.Sec, st.Ctim.Nsec)
	}
	return time.Time{}
}

// systemTrash uses the desktop Trash via GLib's gio, or trash-cli.
func systemTrash(p string) error {
	for _, tool := range [][]string{{"gio", "trash"}, {"trash-put"}} {
		if _, err := exec.LookPath(tool[0]); err == nil {
			return exec.Command(tool[0], append(tool[1:], p)...).Run()
		}
	}
	return errors.New("no trash tool")
}

// reveal asks the file manager to highlight the item, falling back to opening its folder.
func reveal(p string) {
	uri := (&url.URL{Scheme: "file", Path: p}).String()
	if exec.Command("dbus-send", "--session", "--dest=org.freedesktop.FileManager1", "--type=method_call",
		"/org/freedesktop/FileManager1", "org.freedesktop.FileManager1.ShowItems",
		"array:string:"+uri, "string:").Run() == nil {
		return
	}
	dir := p
	if info, err := os.Stat(p); err == nil && !info.IsDir() {
		dir = filepath.Dir(p)
	}
	_ = exec.Command("xdg-open", dir).Start()
}

// OpenBrowser opens the dashboard in the default browser.
func OpenBrowser(url string) { _ = exec.Command("xdg-open", url).Start() }
