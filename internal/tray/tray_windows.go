package tray

import (
	_ "embed"

	"fyne.io/systray"
)

//go:embed icons/tray.ico
var icon []byte

func prepare() {}

func setIcon() { systray.SetIcon(icon) }

// On Windows a left click opens the app and a right click shows the menu.
func onTapped(fn func()) { systray.SetOnTapped(fn) }

func available() bool { return true }

func itemIcon(p []byte) []byte { return pngToICO(p) }
