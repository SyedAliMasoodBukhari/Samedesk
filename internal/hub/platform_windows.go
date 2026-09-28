package hub

import (
	"io/fs"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// hereTime is when the file was created on this PC (Syncthing sets the
// sender's mtime, so this is when it actually arrived).
func hereTime(info fs.FileInfo) time.Time {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.CreationTime.Nanoseconds())
	}
	return time.Time{}
}

func noWindow(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

func systemTrash(p string) error {
	kind := "DeleteFile"
	if info, err := os.Stat(p); err == nil && info.IsDir() {
		kind = "DeleteDirectory"
	}
	ps := "Add-Type -AssemblyName Microsoft.VisualBasic; [Microsoft.VisualBasic.FileIO.FileSystem]::" + kind +
		"($env:HUB_PATH, 'OnlyErrorDialogs', 'SendToRecycleBin')"
	cmd := noWindow(exec.Command("powershell", "-NoProfile", "-Command", ps))
	cmd.Env = append(os.Environ(), "HUB_PATH="+p)
	return cmd.Run()
}

func reveal(p string) { _ = exec.Command("explorer", "/select,", p).Start() }

// OpenBrowser opens the dashboard in the default browser.
func OpenBrowser(url string) {
	_ = noWindow(exec.Command("rundll32", "url.dll,FileProtocolHandler", url)).Start()
}

// DisplayName is the PC's name.
func DisplayName() string {
	h, _ := os.Hostname()
	return h
}
