package hub

import (
	"fmt"
	"net/http"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/update"
)

// What the tray (or any other in-process front end) needs from the hub.

// URL is the dashboard address on this computer.
func (h *Hub) URL() string { return fmt.Sprintf("http://localhost:%d", h.port) }

// Folder is the shared folder on disk.
func (h *Hub) Folder() string { return h.root }

// Status is the current sync state (cached for two seconds).
func (h *Hub) Status() SyncStatus { return h.syncStatus() }

// OpenFolder opens the shared folder in Finder, Explorer or the file manager.
func (h *Hub) OpenFolder() { OpenBrowser(h.root) }

// Idle means nothing is moving: a good moment to restart for an update.
func (h *Hub) Idle() bool {
	s := h.syncStatus()
	if !s.OK || s.State != "idle" || s.Receiving > 0 {
		return false
	}
	for _, p := range s.Peers {
		if p.Connected && p.Need > 0 {
			return false
		}
	}
	return true
}

// updateAPI is the dashboard's Updates section. Only this computer may change
// anything; a phone can see the version.
func (h *Hub) updateAPI(w http.ResponseWriter, r *http.Request) error {
	if h.updates == nil {
		h.writeJSON(w, 200, update.Status{Current: "dev", State: "idle"})
		return nil
	}
	if r.Method != http.MethodGet && !isLoopback(r) {
		return errForbidden
	}
	switch p, m := r.URL.Path, r.Method; {
	case p == "/api/update" && m == http.MethodGet:
	case p == "/api/update/check" && m == http.MethodPost:
		h.updates.Check()
	case p == "/api/update/install" && m == http.MethodPost:
		go h.updates.Install()
	case p == "/api/update/auto" && m == http.MethodPost:
		var b struct {
			Auto bool `json:"auto"`
		}
		if err := readJSON(r, &b); err != nil {
			return err
		}
		h.updates.SetAuto(b.Auto)
	default:
		return errNotFound
	}
	h.writeJSON(w, 200, h.updates.Status())
	return nil
}
