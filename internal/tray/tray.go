// Package tray puts SameDesk in the menu bar (macOS), the notification area
// (Windows) or the status tray (Linux): sync status at a glance, the newest clip
// from another device one click away, and the dashboard behind it.
package tray

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"fyne.io/systray"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/autostart"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/clipboard"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/hub"
)

type Options struct {
	Hub       *hub.Hub
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

type menu struct {
	h                             *hub.Hub
	status, request, latest, send *systray.MenuItem

	mu      sync.Mutex
	tooltip string
	flashAt time.Time
	clip    hub.Incoming // what "latest" copies
	hasClip bool
	peer    string // who "send" goes to, for its title
}

func ready(o Options) {
	setIcon()
	systray.SetTooltip("SameDesk")
	m := &menu{h: o.Hub}

	m.status = systray.AddMenuItem("Starting…", "")
	m.status.Disable()
	m.request = systray.AddMenuItem("", "Open SameDesk to accept or decline")
	m.request.Hide()
	m.latest = systray.AddMenuItem("", "")
	m.latest.Hide()
	m.send = systray.AddMenuItem("Send clipboard", "Put what you copied on the shared clipboard")
	systray.AddSeparator()
	open := systray.AddMenuItem("Open SameDesk", "")
	add := systray.AddMenuItem("Add a device…", "Pair another computer")
	folder := systray.AddMenuItem("Open shared folder", o.Hub.Folder())
	systray.AddSeparator()
	login := systray.AddMenuItemCheckbox("Start at login", "Open SameDesk when you log in", autostart.Enabled())
	quit := systray.AddMenuItem("Quit SameDesk", "Stop syncing until SameDesk is opened again")

	onTapped(func() { hub.OpenBrowser(o.Hub.URL()) })
	m.refresh()

	go func() {
		tick := time.NewTicker(3 * time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				m.refresh()
			case <-systray.TrayOpenedCh: // fresh the moment it's looked at
				m.refresh()
			case <-m.latest.ClickedCh:
				m.copyLatest()
			case <-m.send.ClickedCh:
				m.sendClipboard()
			case <-m.request.ClickedCh:
				hub.OpenBrowser(o.Hub.URL())
			case <-open.ClickedCh:
				hub.OpenBrowser(o.Hub.URL())
			case <-add.ClickedCh:
				hub.OpenBrowser(o.Hub.URL() + "/#add-device")
			case <-folder.ClickedCh:
				o.Hub.OpenFolder()
			case <-login.ClickedCh:
				var err error
				if login.Checked() {
					err = autostart.Disable()
				} else {
					err = autostart.Enable(o.LoginArgs...)
				}
				if err != nil {
					m.flash("Couldn't change that")
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

// ---------- live state ----------

func (m *menu) refresh() {
	s := m.h.Status()
	line, peer := describe(s)
	m.status.SetTitle(line)

	m.mu.Lock()
	m.peer = peer
	m.tooltip = "SameDesk · " + line
	flashing := time.Since(m.flashAt) < 2*time.Second
	m.mu.Unlock()
	if !flashing {
		systray.SetTooltip("SameDesk · " + line)
	}

	if len(s.Pending) > 0 {
		who := s.Pending[0].Name
		if who == "" {
			who = "A new device"
		}
		m.request.SetTitle(who + " wants to connect…")
		m.request.Show()
	} else {
		m.request.Hide()
	}

	if peer != "" {
		m.send.SetTitle("Send clipboard to " + peer)
	} else {
		m.send.SetTitle("Send clipboard")
	}

	in, ok := m.h.LatestIncoming()
	m.mu.Lock()
	m.clip, m.hasClip = in, ok
	m.mu.Unlock()
	if !ok {
		m.latest.Hide()
		return
	}
	title, enabled := latestTitle(in)
	m.latest.SetTitle(title)
	m.latest.SetTooltip("From " + in.Sender + " · " + ago(in.At))
	if enabled {
		m.latest.Enable()
	} else {
		m.latest.Disable()
	}
	m.latest.Show()
}

// describe turns the sync state into one short line, and names the device that
// "Send clipboard" reaches when there is just one.
func describe(s hub.SyncStatus) (line, peer string) {
	var connected []hub.Peer
	for _, p := range s.Peers {
		if p.Connected {
			connected = append(connected, p)
		}
	}
	if len(s.Peers) == 1 {
		peer = s.Peers[0].Name
	}
	busy := s.Receiving > 0
	for _, p := range connected {
		busy = busy || p.Need > 0
	}
	switch {
	case !s.OK:
		return "The sync engine isn't responding", peer
	case s.Paused:
		return "Sync is paused", peer
	case len(s.Peers) == 0:
		return "No devices yet", ""
	case len(connected) == 0 && peer != "":
		return peer + " is offline", peer
	case len(connected) == 0:
		return "Your devices are offline", ""
	case s.Errors > 0:
		return plural(s.Errors, "file") + " couldn't sync", peer
	case peer != "" && !connected[0].Shared:
		return "Waiting for " + peer + " to accept", peer
	case busy && peer != "":
		return "Syncing with " + peer + "…", peer
	case busy:
		return "Syncing…", ""
	case peer != "":
		return "In sync with " + peer, peer
	case len(connected) == len(s.Peers):
		return fmt.Sprintf("In sync with %d devices", len(s.Peers)), ""
	default:
		return fmt.Sprintf("In sync with %d of %d devices", len(connected), len(s.Peers)), ""
	}
}

func latestTitle(in hub.Incoming) (string, bool) {
	from := " from " + in.Sender
	switch {
	case in.Kind == "file" && in.Exists != nil && !*in.Exists:
		return "Receiving “" + short(in.Name, 24) + "”" + from + "…", false
	case in.Kind == "file" && isImage(in.Name):
		return "Copy image" + from + ": “" + short(in.Name, 24) + "”", true
	case in.Kind == "file":
		return "Show file" + from + ": “" + short(in.Name, 24) + "”", true
	case in.Sensitive:
		return "Copy private text" + from, true
	default:
		return "Copy" + from + ": “" + short(firstLine(in.Text), 28) + "”", true
	}
}

// ---------- actions ----------

func (m *menu) copyLatest() {
	m.mu.Lock()
	in, ok := m.clip, m.hasClip
	m.mu.Unlock()
	if !ok {
		return
	}
	var err error
	switch {
	case in.Kind == "file" && isImage(in.Name):
		err = clipboard.WriteImage(in.Local)
	case in.Kind == "file":
		m.h.Reveal(in.Local)
		return
	default:
		err = clipboard.WriteText(in.Text)
	}
	if err != nil {
		m.flash("Couldn't copy")
		return
	}
	m.flash("Copied")
}

func (m *menu) sendClipboard() {
	text, err := clipboard.ReadText()
	switch {
	case err != nil:
		m.flash("Couldn't read the clipboard")
	case strings.TrimSpace(text) == "":
		m.flash("Nothing copied yet")
	default:
		if err := m.h.SendText(text); err != nil {
			m.flash("Couldn't send")
			return
		}
		m.flash("Sent")
		m.refresh()
	}
}

// flash says what just happened, briefly and quietly: next to the icon on macOS,
// in the tooltip elsewhere. No notifications.
func (m *menu) flash(msg string) {
	m.mu.Lock()
	m.flashAt = time.Now()
	at := m.flashAt
	m.mu.Unlock()
	showFlash(msg)
	time.AfterFunc(1800*time.Millisecond, func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.flashAt == at {
			clearFlash(m.tooltip)
		}
	})
}

// ---------- text ----------

func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// short trims to n characters on a word boundary where it can. Menus have no wrapping.
func short(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)[:n]
	if i := strings.LastIndex(string(r), " "); i > n/2 {
		return strings.TrimRight(string(r)[:i], " ,.;:") + "…"
	}
	return string(r) + "…"
}

func isImage(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".tif", ".tiff", ".bmp":
		return true
	}
	return strings.HasPrefix(mime.TypeByExtension(filepath.Ext(name)), "image/") && filepath.Ext(name) != ".svg"
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func ago(ms int64) string {
	d := time.Since(time.UnixMilli(ms))
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	}
	return time.UnixMilli(ms).Format("2 Jan")
}

// Available reports whether this session can show a tray icon at all.
func Available() bool { return available() }
