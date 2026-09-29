package autostart

import (
	"os"
	"path/filepath"
	"strings"
)

func desktopPath() string {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "autostart", "samedesk.desktop")
}

// installed: the .deb/.rpm copy, or an AppImage.
func installed() bool {
	exe, err := executable()
	return os.Getenv("APPIMAGE") != "" || (err == nil && exe == "/usr/bin/samedesk")
}

func Enabled() bool { _, err := os.Stat(desktopPath()); return err == nil }

// quote follows the Desktop Entry spec for the Exec key.
func quote(a string) string {
	if !strings.ContainsAny(a, " \t\"'\\$`") {
		return a
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`")
	return `"` + r.Replace(a) + `"`
}

func Enable(args ...string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	// An AppImage runs from a temporary mount; start the AppImage file itself.
	if img := os.Getenv("APPIMAGE"); img != "" {
		exe = img
	}
	cmd := []string{quote(exe)}
	for _, a := range args {
		cmd = append(cmd, quote(a))
	}
	entry := "[Desktop Entry]\nType=Application\nName=SameDesk\nComment=Shared clipboard, files and notes across your computers\n" +
		"Exec=" + strings.Join(cmd, " ") + "\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"
	if err := os.MkdirAll(filepath.Dir(desktopPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(desktopPath(), []byte(entry), 0o644)
}

func Disable() error {
	if err := os.Remove(desktopPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
