package hub

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// What the tray (or any other in-process front end) needs from the hub.

// URL is the dashboard address on this computer.
func (h *Hub) URL() string { return fmt.Sprintf("http://localhost:%d", h.port) }

// Folder is the shared folder on disk.
func (h *Hub) Folder() string { return h.root }

// Status is the current sync state (cached for two seconds).
func (h *Hub) Status() SyncStatus { return h.syncStatus() }

// Incoming is the newest clip sent from another device, with who sent it.
type Incoming struct {
	ClipOut
	Sender string // "Office PC", or "iPhone" for a phone
	Local  string // full path of a file clip on this computer
}

// LatestIncoming returns the newest clip that did not come from this computer.
func (h *Hub) LatestIncoming() (Incoming, bool) {
	h.mu.Lock()
	clips := h.mergedClips().Clips
	h.mu.Unlock()
	for _, c := range clips { // newest first
		if c.FromID == h.id || (c.FromID == "" && c.From == h.device) {
			continue
		}
		in := Incoming{ClipOut: c, Sender: h.senderLabel(c.Clip)}
		if c.Kind == "file" {
			in.Local = filepath.Join(h.root, filepath.FromSlash(c.Path))
		}
		return in, true
	}
	return Incoming{}, false
}

// senderLabel names a clip's sender the way the dashboard does: the device's own
// name for computers, the kind of phone for phones.
func (h *Hub) senderLabel(c Clip) string {
	if c.FromID == "" || strings.Contains(c.FromID, "/") {
		return c.From
	}
	for _, p := range h.syncStatus().Peers {
		if strings.HasPrefix(p.ID, c.FromID) {
			return p.Name
		}
	}
	return c.From
}

// SendText adds text to the shared clipboard as this computer.
func (h *Hub) SendText(text string) error {
	if strings.TrimSpace(text) == "" {
		return badRequest("Nothing to send")
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.addClip(Clip{ID: newID(), Kind: "text", Text: text, From: h.device, FromID: h.id, At: nowMs()})
}

// Reveal shows a file in Finder, Explorer or the file manager.
func (h *Hub) Reveal(p string) {
	if _, err := os.Stat(p); err == nil {
		reveal(p)
	}
}

// OpenFolder opens the shared folder in Finder, Explorer or the file manager.
func (h *Hub) OpenFolder() { OpenBrowser(h.root) }
