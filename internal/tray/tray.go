// Package tray puts SameDesk in the menu bar (macOS), the notification area
// (Windows) or the status tray (Linux). It stays small on purpose: whether
// you're in sync, and a way into the dashboard, where everything else lives.
package tray

import (
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/autostart"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/hub"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/update"
)

type Options struct {
	Hub       *hub.Hub
	Updates   *update.Updater
	LoginArgs []string // flags to start with at login, so it comes back the same way
	OnExit    func()   // stops the dashboard and the sync engine
}

// Run shows the tray icon and blocks until Quit. It must be called from the main
// goroutine, before anything else claims the main thread.
func Run(o Options) {
	prepare()
	var once sync.Once
	exit := func() { once.Do(o.OnExit) }
	systray.Run(func() { ready(o) }, exit)
	// On macOS the event loop just stops on Quit, without calling onExit.
	exit()
}

// Quit removes the icon and makes Run return (after OnExit has run).
func Quit() { systray.Quit() }

// Available reports whether this session can show a tray icon at all.
func Available() bool { return available() }

func ready(o Options) {
	setIcon()
	systray.SetTooltip("SameDesk")
	h := o.Hub

	status := systray.AddMenuItem("Starting…", "")
	status.Disable()
	request := systray.AddMenuItem("", "Open SameDesk to accept or decline")
	request.Hide()
	upgrade := systray.AddMenuItem("", "")
	upgrade.Hide()
	systray.AddSeparator()
	open := systray.AddMenuItem("Open SameDesk", "")
	folder := systray.AddMenuItem("Open Shared Folder", h.Folder())
	systray.AddSeparator()
	login := systray.AddMenuItemCheckbox("Start at Login", "", autostart.Enabled())
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit SameDesk", "")

	onTapped(func() { hub.OpenBrowser(h.URL()) })

	shown := state{dot: -1} // nothing drawn yet
	refresh := func() {
		st := h.Status()
		s := describe(st)
		if s.line != shown.line {
			status.SetTitle(s.line)
			systray.SetTooltip("SameDesk · " + s.line)
		}
		if s.dot != shown.dot {
			status.SetIcon(itemIcon(dot(s.dot)))
		}
		shown = s
		if len(st.Pending) > 0 {
			who := st.Pending[0].Name
			if who == "" {
				who = "A new device"
			}
			request.SetTitle(who + " wants to connect…")
			request.Show()
		} else {
			request.Hide()
		}
		showUpdate(upgrade, o.Updates)
	}
	refresh()

	go func() {
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				refresh()
			case <-systray.TrayOpenedCh: // fresh the moment it's looked at
				refresh()
			case <-open.ClickedCh:
				hub.OpenBrowser(h.URL())
			case <-request.ClickedCh:
				hub.OpenBrowser(h.URL())
			case <-upgrade.ClickedCh:
				if st := o.Updates.Status(); st.State == "manual" && st.Page != "" {
					hub.OpenBrowser(st.Page)
				} else {
					go o.Updates.Install()
				}
			case <-folder.ClickedCh:
				h.OpenFolder()
			case <-login.ClickedCh:
				if login.Checked() {
					_ = autostart.Disable()
				} else {
					_ = autostart.Enable(o.LoginArgs...)
				}
				if autostart.Enabled() {
					login.Check()
				} else {
					login.Uncheck()
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// showUpdate offers a new version when there's one to act on.
func showUpdate(item *systray.MenuItem, u *update.Updater) {
	if u == nil {
		return
	}
	st := u.Status()
	switch {
	case !st.Available:
		item.Hide()
		return
	case st.State == "installing":
		item.SetTitle("Updating to " + st.Latest + "…")
		item.Disable()
	case st.State == "downloading":
		item.SetTitle("Downloading SameDesk " + st.Latest + "…")
		item.Disable()
	case st.State == "manual":
		item.SetTitle("SameDesk " + st.Latest + " is available…")
		item.Enable()
	default:
		item.SetTitle("Update to SameDesk " + st.Latest)
		item.Enable()
	}
	item.Show()
}

// state is what the status line shows: a word or two and a coloured dot.
type state struct {
	line string
	dot  tone
}

func describe(s hub.SyncStatus) state {
	connected, busy := 0, s.Receiving > 0
	for _, p := range s.Peers {
		if p.Connected {
			connected++
			busy = busy || p.Need > 0
		}
	}
	switch {
	case !s.OK:
		return state{"Not running", red}
	case s.Errors > 0:
		return state{"Some files couldn't sync", red}
	case s.Paused:
		return state{"Paused", grey}
	case len(s.Peers) == 0:
		return state{"No devices yet", grey}
	case connected == 0:
		return state{"Offline", grey}
	case busy:
		return state{"Syncing…", blue}
	default:
		return state{"Up to date", green}
	}
}
