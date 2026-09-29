package hub

import "fmt"

// What the tray (or any other in-process front end) needs from the hub.

// URL is the dashboard address on this computer.
func (h *Hub) URL() string { return fmt.Sprintf("http://localhost:%d", h.port) }

// Folder is the shared folder on disk.
func (h *Hub) Folder() string { return h.root }

// Status is the current sync state (cached for two seconds).
func (h *Hub) Status() SyncStatus { return h.syncStatus() }

// OpenFolder opens the shared folder in Finder, Explorer or the file manager.
func (h *Hub) OpenFolder() { OpenBrowser(h.root) }
