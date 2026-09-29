package hub

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Hidden files, the same on every computer.
//
// Each OS hides files its own way: macOS and Linux hide names that start with a
// dot, Windows hides by a file attribute and has system files of its own
// (desktop.ini, Thumbs.db). None of that syncs, so a folder hidden on one computer
// shows up on another. SameDesk marks both kinds hidden on this computer, in the
// way its file manager understands, when it starts and whenever something arrives.

// systemFiles are hidden by Windows or macOS but have no dot to say so elsewhere.
var systemFiles = map[string]bool{
	"desktop.ini": true, "thumbs.db": true, "ehthumbs.db": true,
	"$recycle.bin": true, "system volume information": true, "icon\r": true,
}

// hiddenName reports whether a file or folder should be hidden on every computer.
func hiddenName(name string) bool {
	return strings.HasPrefix(name, ".") || systemFiles[strings.ToLower(name)]
}

// hideAll marks everything in the folder that should be hidden. What is inside a
// hidden folder is already out of sight, so it isn't visited.
func (h *Hub) hideAll() {
	var found []string
	_ = filepath.WalkDir(h.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == h.root {
			return nil
		}
		if hiddenName(d.Name()) {
			found = append(found, p)
			if d.IsDir() {
				return fs.SkipDir
			}
		}
		return nil
	})
	hideNative(found)
}

// hidePaths marks the first hidden part of each folder-relative path, so both
// ".notes/today.md" and "Photos/Thumbs.db" end up hidden.
func (h *Hub) hidePaths(rels []string) {
	var found []string
	seen := map[string]bool{}
	for _, rel := range rels {
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for i, part := range parts {
			if !hiddenName(part) {
				continue
			}
			p := filepath.Join(append([]string{h.root}, parts[:i+1]...)...)
			if !seen[p] {
				seen[p] = true
				if _, err := os.Lstat(p); err == nil {
					found = append(found, p)
				}
			}
			break
		}
	}
	hideNative(found)
}

// hideLoop hides what's there now, then follows the sync engine's change events:
// files arriving from other devices and files created here.
func (h *Hub) hideLoop() {
	h.hideAll()
	client := &http.Client{Timeout: 90 * time.Second}
	since := 0
	for {
		q := url.Values{"events": {"LocalChangeDetected,RemoteChangeDetected"}, "since": {strconv.Itoa(since)}, "timeout": {"60"}}
		req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(h.eng.APIURL, "/")+"/rest/events?"+q.Encode(), nil)
		req.Header.Set("X-API-Key", h.eng.APIKey)
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		var evs []struct {
			ID   int `json:"id"`
			Data struct {
				Folder string `json:"folder"`
				Action string `json:"action"`
				Path   string `json:"path"`
			} `json:"data"`
		}
		err = json.NewDecoder(resp.Body).Decode(&evs)
		resp.Body.Close()
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		var paths []string
		for _, e := range evs {
			since = e.ID
			if e.Data.Folder == h.eng.FolderID && e.Data.Action != "deleted" {
				paths = append(paths, e.Data.Path)
			}
		}
		h.hidePaths(paths)
	}
}
