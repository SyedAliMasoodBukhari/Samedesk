package tray

import (
	_ "embed"
	"os"
	"path/filepath"

	"fyne.io/systray"
)

//go:embed icons/tray.png
var icon []byte

func prepare() {}

func setIcon() { systray.SetIcon(icon) }

// Linux trays differ in what a click does; leave it to the desktop (usually the menu).
func onTapped(func()) {}

// The tray talks StatusNotifierItem over the session bus, so it needs a desktop
// session: a display and a D-Bus session bus. Servers run without the icon.
func available() bool {
	if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		return true
	}
	_, err := os.Stat(filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "bus"))
	return os.Getenv("XDG_RUNTIME_DIR") != "" && err == nil
}

func itemIcon(p []byte) []byte { return p }
