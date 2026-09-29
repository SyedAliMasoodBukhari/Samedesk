// Package hub is the SameDesk dashboard: a small web app for the shared folder
// with a clipboard, files and notes. It serves the embedded UI on localhost, and
// to phones on the same Wi-Fi that open the key link shown in Settings.
package hub

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/engine"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/pairing"
	"github.com/SyedAliMasoodBukhari/samedesk/web"
)

const (
	inbox        = "Inbox" // where files sent through the clipboard land
	maxClips     = 300
	sensitiveTTL = 10 * 60 * 1000
)

// Config for a Hub.
type Config struct {
	Root    string // the shared folder
	DataDir string // per-device state that is never synced
	Port    int
	Engine  *engine.Engine
}

type Hub struct {
	root, hubDir, local string
	port                int
	device              string // "Mac", "Windows" or "Linux"
	id                  string // short Syncthing device ID, unique per device
	key                 string
	build               string
	eng                 *engine.Engine
	beacon              *pairing.Beacon

	mu    sync.Mutex
	clips *store[clipDoc]
	notes *store[noteDoc]
	sync  syncCache

	seen     atomic.Int64    // when a visible dashboard on this computer last checked in (unix ms)
	prompted map[string]bool // pairing requests already brought to the user's attention
}

// Device is what kind of computer this is. SAMEDESK_DEVICE_KIND overrides it,
// so one machine can stand in for another OS when testing or making screenshots.
func Device() string {
	if k := os.Getenv("SAMEDESK_DEVICE_KIND"); k == "Mac" || k == "Windows" || k == "Linux" {
		return k
	}
	switch runtime.GOOS {
	case "darwin":
		return "Mac"
	case "windows":
		return "Windows"
	default:
		return "Linux"
	}
}

func New(c Config) (*Hub, error) {
	root, err := filepath.EvalSymlinks(c.Root)
	if err != nil {
		return nil, err
	}
	h := &Hub{root: root, hubDir: filepath.Join(root, ".samedesk"), local: c.DataDir, port: c.Port, device: Device(), id: c.Engine.ID.Short().String(),
		build: strconv.FormatInt(time.Now().Unix(), 10), eng: c.Engine}
	if err := os.MkdirAll(h.local, 0o700); err != nil {
		return nil, err
	}
	if h.key, err = loadKey(filepath.Join(h.local, "key")); err != nil {
		return nil, err
	}
	// Files are named by device ID, so two Macs (or two PCs) never write the same file.
	if h.clips, err = openStore(filepath.Join(h.hubDir, "clips"), h.id, func() clipDoc { return clipDoc{} }, fillClipDoc); err != nil {
		return nil, err
	}
	if h.notes, err = openStore(filepath.Join(h.hubDir, "notes"), h.id, func() noteDoc { return noteDoc{} }, fillNoteDoc); err != nil {
		return nil, err
	}
	// Each device writes who it is into its own files, so the others always show
	// its current name, even after the computer is renamed.
	full, name := c.Engine.ID.String(), c.Engine.Name()
	if m := &h.clips.mine; m.Device != h.device || m.ID != full || m.Name != name {
		m.Device, m.ID, m.Name = h.device, full, name
		_ = h.clips.save()
		h.rescan(".samedesk/clips")
	}
	if m := &h.notes.mine; m.Device != h.device || m.ID != full || m.Name != name {
		m.Device, m.ID, m.Name = h.device, full, name
		_ = h.notes.save()
		h.rescan(".samedesk/notes")
	}
	h.beacon = pairing.Start(c.Engine.ID.String(), c.Engine.Name(), h.device)
	h.beacon.SetPort(c.Engine.SyncPort())
	h.prompted = map[string]bool{}
	go h.pairWatch()
	go h.expiryLoop()
	go h.hideLoop()
	return h, nil
}

func loadKey(p string) (string, error) {
	if b, err := os.ReadFile(p); err == nil && len(strings.TrimSpace(string(b))) > 0 {
		return strings.TrimSpace(string(b)), nil
	}
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	k := base64.RawURLEncoding.EncodeToString(b)
	return k, os.WriteFile(p, []byte(k), 0o600)
}

// ---------- listening ----------

// Server owns the dashboard socket. Binding it first doubles as the single-instance check.
type Server struct {
	ln  net.Listener
	srv *http.Server
}

func Listen(port int) (*Server, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		return nil, err
	}
	return &Server{ln: ln}, nil
}

func (s *Server) Serve(h http.Handler) error {
	s.srv = &http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}
	err := s.srv.Serve(s.ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Close() error {
	if s.srv != nil {
		return s.srv.Close()
	}
	return s.ln.Close()
}

// ---------- request plumbing ----------

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

var (
	errForbidden = httpError{403, "Not allowed"}
	errNotFound  = httpError{404, "Not found"}
)

func badRequest(msg string) error { return httpError{400, msg} }

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (h *Hub) sender(r *http.Request) string {
	if isLoopback(r) {
		return h.device
	}
	return deviceOf(r.UserAgent())
}

// senderID identifies who is acting: this computer, or a phone using this computer's hub.
func (h *Hub) senderID(r *http.Request) string {
	if isLoopback(r) {
		return h.id
	}
	return h.id + "/" + deviceOf(r.UserAgent())
}

func deviceOf(ua string) string {
	for _, p := range [][2]string{{"iPhone", "iPhone"}, {"iPad", "iPad"}, {"Android", "Android"},
		{"Windows", "Windows"}, {"Macintosh", "Mac"}, {"Linux", "Linux"}, {"CrOS", "Linux"}} {
		if strings.Contains(ua, p[0]) {
			return p[1]
		}
	}
	return "Phone"
}

func (h *Hub) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Build", h.build)
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	if err := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 8<<20)).Decode(v); err != nil {
		return badRequest("Bad request")
	}
	return nil
}

func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	local := isLoopback(r)
	if local {
		// Only this computer's own browser: refuse DNS-rebinding hosts.
		if r.Host != fmt.Sprintf("localhost:%d", h.port) && r.Host != fmt.Sprintf("127.0.0.1:%d", h.port) {
			h.writeJSON(w, 403, map[string]string{"error": "Forbidden"})
			return
		}
	} else {
		// Phones need the key link once; it sets a cookie.
		if r.Method == http.MethodGet && r.URL.Query().Get("k") == h.key {
			http.SetCookie(w, &http.Cookie{Name: "hub", Value: h.key, Path: "/", MaxAge: 31536000, HttpOnly: true, SameSite: http.SameSiteLaxMode})
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		if c, err := r.Cookie("hub"); err != nil || c.Value != h.key {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(401)
			fmt.Fprint(w, "<p style='font:16px system-ui;padding:40px'>Scan the QR code in SameDesk on your computer to connect.</p>")
			return
		}
	}
	// A custom header on the API, so other websites can't fire requests at it.
	if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("X-Hub") != "1" {
		h.writeJSON(w, 403, map[string]string{"error": "Forbidden"})
		return
	}
	if isLoopback(r) && r.Header.Get("X-Visible") == "1" {
		h.seen.Store(nowMs())
	}
	if err := h.route(w, r); err != nil {
		var he httpError
		if !errors.As(err, &he) {
			switch {
			case errors.Is(err, fs.ErrNotExist):
				he = errNotFound
			case errors.Is(err, fs.ErrExist):
				he = httpError{400, "A file with that name already exists"}
			default:
				he = httpError{500, "Something went wrong"}
			}
		}
		h.writeJSON(w, he.code, map[string]string{"error": he.msg})
	}
}

func (h *Hub) route(w http.ResponseWriter, r *http.Request) error {
	p, m := r.URL.Path, r.Method
	switch {
	case p == "/" && m == http.MethodGet:
		b, _ := web.FS.ReadFile("index.html")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Build", h.build)
		_, _ = w.Write(b)
		return nil
	case strings.HasPrefix(p, "/assets/") && (m == http.MethodGet || m == http.MethodHead):
		b, err := web.FS.ReadFile("assets/" + path.Base(p))
		if err != nil {
			return errNotFound
		}
		w.Header().Set("Content-Type", mime.TypeByExtension(path.Ext(p)))
		w.Header().Set("Cache-Control", "public, max-age=604800")
		_, _ = w.Write(b)
		return nil
	case p == "/api/me":
		return h.me(w, r)
	case p == "/api/sync":
		h.writeJSON(w, 200, h.syncStatus())
		return nil
	case p == "/api/clips" || strings.HasPrefix(p, "/api/clips/"):
		return h.clipsAPI(w, r)
	case p == "/api/pair" || strings.HasPrefix(p, "/api/pair/") || p == "/api/devices/remove":
		return h.pairAPI(w, r)
	case p == "/api/notes" || strings.HasPrefix(p, "/api/notes/"):
		return h.notesAPI(w, r)
	case strings.HasPrefix(p, "/f/") && (m == http.MethodGet || m == http.MethodHead):
		return h.serveFile(w, r)
	case strings.HasPrefix(p, "/api/"):
		return h.filesAPI(w, r)
	}
	return errNotFound
}

func (h *Hub) me(w http.ResponseWriter, r *http.Request) error {
	local := isLoopback(r)
	out := map[string]any{"device": h.sender(r), "id": h.senderID(r), "name": h.eng.Name(), "host": h.device, "local": local, "folder": filepath.Base(h.root), "link": nil}
	if ip := lanIP(); ip != "" && local {
		out["link"] = fmt.Sprintf("http://%s:%d/?k=%s", ip, h.port, h.key)
	}
	h.writeJSON(w, 200, out)
	return nil
}

func lanIP() string {
	c, err := net.Dial("udp", "10.255.255.255:1")
	if err != nil {
		return ""
	}
	defer c.Close()
	return c.LocalAddr().(*net.UDPAddr).IP.String()
}

// ---------- clipboard ----------

func (h *Hub) mergedClips() ClipState {
	docs, t := h.clips.all(), nowMs()
	deleted := map[string]bool{}
	pins := map[string]Pin{}
	for _, d := range docs {
		for _, id := range d.Deleted {
			deleted[id] = true
		}
		for id, p := range d.Pins {
			if cur, ok := pins[id]; !ok || p.At > cur.At {
				pins[id] = p
			}
		}
	}
	out := []ClipOut{}
	for _, d := range docs {
		for _, c := range d.Clips {
			if deleted[c.ID] || (c.Expires != 0 && c.Expires < t) {
				continue
			}
			o := ClipOut{Clip: c, Pinned: pins[c.ID].On}
			if c.Kind == "file" {
				_, err := os.Stat(filepath.Join(h.root, filepath.FromSlash(c.Path)))
				ok := err == nil
				o.Exists = &ok
			}
			out = append(out, o)
		}
	}
	sortClips(out)
	return ClipState{Version: version(out), Clips: out}
}

func sortClips(c []ClipOut) {
	for i := 1; i < len(c); i++ { // insertion sort keeps equal timestamps stable
		for j := i; j > 0 && c[j].At > c[j-1].At; j-- {
			c[j], c[j-1] = c[j-1], c[j]
		}
	}
}

func (h *Hub) saveClips() error {
	others := map[string]bool{}
	for _, d := range h.clips.others() {
		for _, c := range d.Clips {
			others[c.ID] = true
		}
	}
	m, t := &h.clips.mine, nowMs()
	kept := m.Clips[:0]
	own := map[string]bool{}
	for _, c := range m.Clips {
		if c.Expires == 0 || c.Expires >= t { // drop expired private clips
			kept = append(kept, c)
			own[c.ID] = true
		}
	}
	m.Clips = kept
	del := m.Deleted[:0]
	for _, id := range m.Deleted {
		if others[id] { // prune stale tombstones
			del = append(del, id)
		}
	}
	m.Deleted = del
	for id := range m.Pins {
		if !others[id] && !own[id] {
			delete(m.Pins, id)
		}
	}
	if err := h.clips.save(); err != nil {
		return err
	}
	h.rescan(".samedesk/clips")
	return nil
}

// addClip puts a new clip of this device's on top. The caller holds h.mu.
func (h *Hub) addClip(c Clip) error {
	m := &h.clips.mine
	m.Clips = append([]Clip{c}, m.Clips...)
	if len(m.Clips) > maxClips {
		m.Clips = m.Clips[:maxClips]
	}
	return h.saveClips()
}

func (h *Hub) deleteClip(id string) {
	live := h.mergedClips().Clips
	var clip *ClipOut
	for i := range live {
		if live[i].ID == id {
			clip = &live[i]
		}
	}
	m := &h.clips.mine
	before := len(m.Clips)
	kept := m.Clips[:0]
	for _, c := range m.Clips {
		if c.ID != id {
			kept = append(kept, c)
		}
	}
	m.Clips = kept
	if len(m.Clips) == before && !contains(m.Deleted, id) {
		m.Deleted = append(m.Deleted, id) // another device's clip: hide it everywhere
	}
	// A file sent through the clipboard lives in Inbox only for that clip, so it goes too
	// (Syncthing then removes it on the other devices). Files elsewhere are never touched.
	if clip != nil && clip.Kind == "file" && strings.HasPrefix(clip.Path, inbox+"/") {
		for _, c := range live {
			if c.ID != id && c.Kind == "file" && c.Path == clip.Path {
				return
			}
		}
		full := filepath.Join(h.root, filepath.FromSlash(clip.Path))
		if _, err := os.Stat(full); err == nil {
			_ = h.toTrash(full)
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func newID() string {
	b := make([]byte, 5)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

func (h *Hub) clipsAPI(w http.ResponseWriter, r *http.Request) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/clips"), "/")
	switch {
	case id == "" && r.Method == http.MethodGet:
	case id == "" && r.Method == http.MethodPost:
		var b struct {
			Text      string `json:"text"`
			Sensitive bool   `json:"sensitive"`
			File      string `json:"file"`
		}
		if err := readJSON(r, &b); err != nil {
			return err
		}
		c := Clip{ID: newID(), From: h.sender(r), FromID: h.senderID(r), At: nowMs()}
		if b.File != "" {
			full, err := h.safePath(b.File)
			if err != nil {
				return err
			}
			info, err := os.Stat(full)
			if err != nil {
				return err
			}
			c.Kind, c.Path, c.Name, c.Size = "file", h.rel(full), filepath.Base(full), info.Size()
		} else {
			if strings.TrimSpace(b.Text) == "" {
				return badRequest("Nothing to send")
			}
			c.Kind, c.Text = "text", b.Text
			if b.Sensitive {
				c.Sensitive, c.Expires = true, c.At+sensitiveTTL
			}
		}
		if err := h.addClip(c); err != nil {
			return err
		}
	case id == "" && r.Method == http.MethodDelete: // clear all, keeping pinned
		for _, c := range h.mergedClips().Clips {
			if !c.Pinned {
				h.deleteClip(c.ID)
			}
		}
		if err := h.saveClips(); err != nil {
			return err
		}
	case id != "" && r.Method == http.MethodDelete:
		h.deleteClip(id)
		if err := h.saveClips(); err != nil {
			return err
		}
	case id != "" && r.Method == http.MethodPatch:
		var b struct {
			Pinned bool `json:"pinned"`
		}
		if err := readJSON(r, &b); err != nil {
			return err
		}
		h.clips.mine.Pins[id] = Pin{On: b.Pinned, At: float64(time.Now().UnixNano()) / 1e9}
		if err := h.saveClips(); err != nil {
			return err
		}
	default:
		return errNotFound
	}
	h.writeJSON(w, 200, h.mergedClips())
	return nil
}

func (h *Hub) expiryLoop() {
	for range time.Tick(30 * time.Second) {
		h.mu.Lock()
		t := nowMs()
		for _, c := range h.clips.mine.Clips {
			if c.Expires != 0 && c.Expires < t {
				_ = h.saveClips()
				break
			}
		}
		h.mu.Unlock()
	}
}

// ---------- notes ----------

func (h *Hub) notesAPI(w http.ResponseWriter, r *http.Request) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/api/notes"), "/")
	switch {
	case id == "" && r.Method == http.MethodGet:
		h.writeJSON(w, 200, mergeNotes(h.notes.all()))
	case id == "" && r.Method == http.MethodPost:
		var b struct{ ID, Title, Body string }
		if err := readJSON(r, &b); err != nil {
			return err
		}
		if b.ID == "" {
			b.ID = newID()
		}
		if len([]rune(b.Title)) > 200 {
			b.Title = string([]rune(b.Title)[:200])
		}
		t, by := nowMs(), h.sender(r)
		n := Note{ID: b.ID, Title: b.Title, Body: b.Body, Updated: t, By: by, ByID: h.senderID(r), Created: t, CreatedBy: by}
		for _, prev := range mergeNotes(h.notes.all()).Notes {
			if prev.ID == b.ID {
				n.Created, n.CreatedBy = prev.Created, prev.CreatedBy
			}
		}
		h.notes.mine.Notes[b.ID] = n
		if err := h.notes.save(); err != nil {
			return err
		}
		h.rescan(".samedesk/notes")
		st := mergeNotes(h.notes.all())
		st.Note = &n
		h.writeJSON(w, 200, st)
	case id != "" && r.Method == http.MethodDelete:
		delete(h.notes.mine.Notes, id)
		h.notes.mine.Deleted[id] = nowMs()
		if err := h.notes.save(); err != nil {
			return err
		}
		h.rescan(".samedesk/notes")
		h.writeJSON(w, 200, mergeNotes(h.notes.all()))
	default:
		return errNotFound
	}
	return nil
}
