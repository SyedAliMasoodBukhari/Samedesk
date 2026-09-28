package hub

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var hidden = map[string]bool{"desktop.ini": true, "Thumbs.db": true, "Shared Hub.url": true}

func visible(name string) bool {
	return !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "~") && !hidden[name]
}

// safePath resolves a folder-relative path, refusing anything outside the folder or inside .hub.
func (h *Hub) safePath(rel string) (string, error) {
	rel = strings.Trim(filepath.FromSlash(rel), `/\`)
	full := filepath.Clean(filepath.Join(h.root, rel))
	if real, err := filepath.EvalSymlinks(full); err == nil {
		full = real
	}
	if full != h.root && !strings.HasPrefix(full, h.root+string(os.PathSeparator)) {
		return "", errForbidden
	}
	if full == h.hubDir || strings.HasPrefix(full, h.hubDir+string(os.PathSeparator)) {
		return "", errForbidden
	}
	return full, nil
}

func (h *Hub) rel(full string) string {
	r, err := filepath.Rel(h.root, full)
	if err != nil || r == "." {
		return ""
	}
	return filepath.ToSlash(r)
}

func unique(p string) string {
	if _, err := os.Lstat(p); err != nil {
		return p
	}
	ext := filepath.Ext(p)
	base := strings.TrimSuffix(p, ext)
	for n := 2; ; n++ {
		c := base + " (" + strconv.Itoa(n) + ")" + ext
		if _, err := os.Lstat(c); err != nil {
			return c
		}
	}
}

var badChars = regexp.MustCompile(`[\\/:*?"<>|\x00-\x1f]`)

func cleanName(name string) (string, error) {
	name = strings.Trim(badChars.ReplaceAllString(strings.TrimSpace(name), "_"), ". ")
	if name == "" {
		return "", badRequest("bad name")
	}
	return name, nil
}

// Item is a file or folder as the dashboard sees it.
type Item struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Dir     bool   `json:"dir"`
	Mtime   int64  `json:"mtime"`
	Arrived int64  `json:"arrived"`
	Size    *int64 `json:"size"`
	Count   *int   `json:"count,omitempty"`
	Newest  *int64 `json:"newest,omitempty"`
}

// arrived is when a file showed up on this device. Syncthing keeps the sender's
// mtime, so also look at when it was written here.
func arrived(info fs.FileInfo) int64 {
	t := info.ModTime()
	if h := hereTime(info); h.After(t) {
		t = h
	}
	return t.UnixMilli()
}

// walk visits visible entries below top (skipping .hub and hidden items), up to limit.
func walk(top string, limit int, fn func(path string, d fs.DirEntry)) {
	n := 0
	_ = filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
		if err != nil || p == top {
			return nil
		}
		if !visible(d.Name()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if n++; n > limit {
			return filepath.SkipAll
		}
		fn(p, d)
		return nil
	})
}

func (h *Hub) item(full string, d fs.DirEntry, withNewest bool) (Item, bool) {
	info, err := d.Info()
	if err != nil {
		return Item{}, false
	}
	it := Item{Name: d.Name(), Path: h.rel(full), Dir: d.IsDir(), Mtime: info.ModTime().UnixMilli(), Arrived: arrived(info)}
	if !it.Dir {
		s := info.Size()
		it.Size = &s
		return it, true
	}
	count := 0
	if entries, err := os.ReadDir(full); err == nil {
		for _, e := range entries {
			if visible(e.Name()) {
				count++
			}
		}
	}
	it.Count = &count
	if withNewest {
		var newest int64
		walk(full, 3000, func(p string, d fs.DirEntry) {
			if i, err := d.Info(); err == nil {
				if a := arrived(i); a > newest {
					newest = a
				}
			}
		})
		it.Newest = &newest
	}
	return it, true
}

func (h *Hub) filesAPI(w http.ResponseWriter, r *http.Request) error {
	q := r.URL.Query()
	switch p, m := r.URL.Path, r.Method; {
	case p == "/api/files" && m == http.MethodGet:
		d, err := h.safePath(q.Get("path"))
		if err != nil {
			return err
		}
		entries, err := os.ReadDir(d)
		if err != nil {
			return err
		}
		items := []Item{}
		for _, e := range entries {
			if visible(e.Name()) {
				if it, ok := h.item(filepath.Join(d, e.Name()), e, true); ok {
					items = append(items, it)
				}
			}
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Dir != items[j].Dir {
				return items[i].Dir
			}
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		})
		h.writeJSON(w, 200, map[string]any{"path": h.rel(d), "items": items})

	case p == "/api/recent" && m == http.MethodGet:
		items := []Item{}
		walk(h.root, 20000, func(p string, d fs.DirEntry) {
			if !d.IsDir() {
				if it, ok := h.item(p, d, false); ok {
					items = append(items, it)
				}
			}
		})
		sort.Slice(items, func(i, j int) bool { return items[i].Arrived > items[j].Arrived })
		if len(items) > 40 {
			items = items[:40]
		}
		h.writeJSON(w, 200, map[string]any{"items": items})

	case p == "/api/search" && m == http.MethodGet:
		words := strings.Fields(strings.ToLower(q.Get("q")))
		items := []Item{}
		if len(words) > 0 {
			walk(h.root, 20000, func(p string, d fs.DirEntry) {
				name := strings.ToLower(d.Name())
				for _, w := range words {
					if !strings.Contains(name, w) {
						return
					}
				}
				if it, ok := h.item(p, d, false); ok {
					items = append(items, it)
				}
			})
		}
		sort.Slice(items, func(i, j int) bool {
			if items[i].Dir != items[j].Dir {
				return !items[i].Dir
			}
			return items[i].Arrived > items[j].Arrived
		})
		if len(items) > 150 {
			items = items[:150]
		}
		h.writeJSON(w, 200, map[string]any{"items": items})

	case p == "/api/upload" && m == http.MethodPut:
		folder := q.Get("path")
		if q.Get("inbox") != "" {
			if err := os.MkdirAll(filepath.Join(h.root, inbox), 0o755); err != nil {
				return err
			}
			folder = inbox
		}
		d, err := h.safePath(folder)
		if err != nil {
			return err
		}
		name, err := cleanName(q.Get("name"))
		if err != nil {
			return err
		}
		// Write outside the folder first, so Syncthing never sees half a file.
		tmp, err := os.CreateTemp(h.local, "upload-*")
		if err != nil {
			return err
		}
		_, err = io.Copy(tmp, r.Body)
		tmp.Close()
		if err != nil {
			os.Remove(tmp.Name())
			return err
		}
		dest := unique(filepath.Join(d, name))
		if err := moveFile(tmp.Name(), dest); err != nil {
			os.Remove(tmp.Name())
			return err
		}
		h.rescan(h.rel(dest))
		h.writeJSON(w, 200, map[string]string{"name": filepath.Base(dest), "path": h.rel(dest)})

	case p == "/api/mkdir" && m == http.MethodPost:
		var b struct{ Path, Name string }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		parent, err := h.safePath(b.Path)
		if err != nil {
			return err
		}
		name, err := cleanName(b.Name)
		if err != nil {
			return err
		}
		dest := unique(filepath.Join(parent, name))
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		h.rescan(h.rel(dest))
		h.writeJSON(w, 200, map[string]string{"name": filepath.Base(dest)})

	case p == "/api/rename" && m == http.MethodPost:
		var b struct{ Path, Name string }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		src, err := h.safePath(b.Path)
		if err != nil || src == h.root {
			return errForbidden
		}
		name, err := cleanName(b.Name)
		if err != nil {
			return err
		}
		dest := filepath.Join(filepath.Dir(src), name)
		if _, err := os.Lstat(dest); err == nil && !strings.EqualFold(dest, src) {
			return badRequest("A file with that name already exists")
		}
		if err := os.Rename(src, dest); err != nil {
			return err
		}
		h.rescan(h.rel(filepath.Dir(src)))
		h.writeJSON(w, 200, map[string]any{})

	case p == "/api/delete" && m == http.MethodPost:
		var b struct{ Path string }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		src, err := h.safePath(b.Path)
		if err != nil || src == h.root {
			return errForbidden
		}
		if err := h.toTrash(src); err != nil {
			return err
		}
		h.writeJSON(w, 200, map[string]any{})

	case p == "/api/reveal" && m == http.MethodPost:
		if !isLoopback(r) {
			return errForbidden
		}
		var b struct{ Path string }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		full, err := h.safePath(b.Path)
		if err != nil {
			return err
		}
		reveal(full)
		h.writeJSON(w, 200, map[string]any{})

	default:
		return errNotFound
	}
	return nil
}

func (h *Hub) serveFile(w http.ResponseWriter, r *http.Request) error {
	rel, err := url.PathUnescape(strings.TrimPrefix(r.URL.EscapedPath(), "/f/"))
	if err != nil {
		return badRequest("Bad path")
	}
	full, err := h.safePath(rel)
	if err != nil {
		return err
	}
	info, err := os.Stat(full)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errNotFound
	}
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	ctype := mime.TypeByExtension(filepath.Ext(full))
	if ctype == "" {
		ctype = "application/octet-stream"
	} else if strings.HasPrefix(ctype, "text/") && !strings.Contains(ctype, "charset") {
		ctype += "; charset=utf-8"
	}
	disp := "inline"
	if r.URL.Query().Has("dl") {
		disp = "attachment"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", disp+"; filename*=UTF-8''"+url.PathEscape(filepath.Base(full)))
	http.ServeContent(w, r, "", info.ModTime(), f) // handles ranges, so videos can seek
	return nil
}

// moveFile renames, or copies when the data dir is on a different volume.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
}

// toTrash sends to the system Trash / Recycle Bin, falling back to a local trash folder.
func (h *Hub) toTrash(p string) error {
	defer h.rescan(h.rel(filepath.Dir(p)))
	if err := systemTrash(p); err == nil {
		if _, err := os.Lstat(p); os.IsNotExist(err) {
			return nil
		}
	}
	dir := filepath.Join(h.local, "trash")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return moveFile(p, unique(filepath.Join(dir, filepath.Base(p))))
}
