package tray

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

// A menu bar app: no Dock icon, no app menu, and it never takes focus on launch.
static void samedesk_accessory(void) {
	[[NSApplication sharedApplication] setActivationPolicy:NSApplicationActivationPolicyAccessory];
}
*/
import "C"

import (
	_ "embed"

	"fyne.io/systray"
)

var (
	//go:embed icons/tray-template@2x.png
	templateIcon []byte
	//go:embed icons/tray.png
	colourIcon []byte
)

// prepare runs on the main thread, before the event loop starts.
func prepare() { C.samedesk_accessory() }

// A template image follows the menu bar: black on light, white on dark, tinted when open.
func setIcon() { systray.SetTemplateIcon(templateIcon, colourIcon) }

// On macOS a click opens the menu, like every other menu bar item.
func onTapped(func()) {}

func available() bool { return true }

func itemIcon(p []byte) []byte { return p }
