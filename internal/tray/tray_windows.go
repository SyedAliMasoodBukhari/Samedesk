package tray

import (
	_ "embed"
	"time"
	"unsafe"

	"fyne.io/systray"
	"golang.org/x/sys/windows"
)

//go:embed icons/tray.ico
var icon []byte

var findWindow = windows.NewLazySystemDLL("user32.dll").NewProc("FindWindowW")

// prepare waits for the taskbar. At login SameDesk can start before it exists,
// and the tray icon is only ever added once, so it would never appear.
func prepare() {
	class, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	for waited := 0; waited < 300; waited++ {
		if h, _, _ := findWindow.Call(uintptr(unsafe.Pointer(class)), 0); h != 0 {
			if waited > 0 {
				time.Sleep(3 * time.Second) // it exists; give the notification area a moment
			}
			return
		}
		time.Sleep(time.Second)
	}
}

func setIcon() { systray.SetIcon(icon) }

// On Windows a left click opens the app and a right click shows the menu.
func onTapped(fn func()) { systray.SetOnTapped(fn) }

func available() bool { return true }

func itemIcon(p []byte) []byte { return pngToICO(p) }
