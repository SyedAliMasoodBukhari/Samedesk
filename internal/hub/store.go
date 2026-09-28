package hub

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// Shared data (clipboard, notes) lives in <folder>/.hub/<kind>/<device short ID>.json.
// Each device writes only its own file and merges the others on read, so
// Syncthing carries everything across without ever seeing conflicting edits.

var storeFile = regexp.MustCompile(`^\w+\.json$`) // skips temp files and Syncthing conflict copies

type cached[T any] struct {
	mtime time.Time
	doc   T
}

type store[T any] struct {
	dir, device string
	mine        T
	cache       map[string]cached[T]
}

func openStore[T any](dir, device string, empty func() T, fill func(*T)) (*store[T], error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &store[T]{dir: dir, device: device, mine: empty(), cache: map[string]cached[T]{}}
	if b, err := os.ReadFile(s.path()); err == nil {
		_ = json.Unmarshal(b, &s.mine)
	}
	fill(&s.mine)
	return s, nil
}

func (s *store[T]) path() string { return filepath.Join(s.dir, s.device+".json") }

func (s *store[T]) others() []T {
	var docs []T
	for _, d := range s.named() {
		docs = append(docs, d)
	}
	return docs
}

// named returns the other devices' documents keyed by file name (their short device ID).
func (s *store[T]) named() map[string]T {
	docs := map[string]T{}
	entries, _ := os.ReadDir(s.dir)
	for _, e := range entries {
		if !storeFile.MatchString(e.Name()) || e.Name() == s.device+".json" {
			continue
		}
		p := filepath.Join(s.dir, e.Name())
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		c, ok := s.cache[p]
		if !ok || !c.mtime.Equal(info.ModTime()) {
			var doc T
			b, err := os.ReadFile(p)
			if err == nil && json.Unmarshal(b, &doc) == nil {
				c = cached[T]{info.ModTime(), doc}
				s.cache[p] = c
			} else if !ok {
				continue // mid-sync read and nothing cached yet
			} // else keep the last good copy
		}
		docs[e.Name()[:len(e.Name())-5]] = c.doc
	}
	return docs
}

func (s *store[T]) all() []T { return append([]T{s.mine}, s.others()...) }

func (s *store[T]) save() error {
	b, err := json.Marshal(s.mine)
	if err != nil {
		return err
	}
	return os.WriteFile(s.path(), b, 0o644)
}

// ---------- clipboard ----------

type Clip struct {
	ID        string `json:"id"`
	Kind      string `json:"kind,omitempty"` // "text" or "file"; older clips have none
	Text      string `json:"text,omitempty"`
	Name      string `json:"name,omitempty"`
	Path      string `json:"path,omitempty"`
	Size      int64  `json:"size,omitempty"`
	From      string `json:"from"`             // label shown in the UI, e.g. "Windows" or "iPhone"
	FromID    string `json:"fromId,omitempty"` // who sent it, e.g. "BAFDLNP" or "BAFDLNP/iPhone"
	At        int64  `json:"at"`
	Sensitive bool   `json:"sensitive,omitempty"`
	Expires   int64  `json:"expires,omitempty"`
}

// Pin is stored as [on, unixSeconds] to stay compatible with existing files.
type Pin struct {
	On bool
	At float64
}

func (p Pin) MarshalJSON() ([]byte, error) { return json.Marshal([]any{p.On, p.At}) }
func (p *Pin) UnmarshalJSON(b []byte) error {
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil || len(raw) != 2 {
		return err
	}
	_ = json.Unmarshal(raw[0], &p.On)
	return json.Unmarshal(raw[1], &p.At)
}

type clipDoc struct {
	Device  string         `json:"device,omitempty"`   // "Mac", "Windows" or "Linux"
	ID      string         `json:"deviceId,omitempty"` // full Syncthing device ID
	Clips   []Clip         `json:"clips"`
	Deleted []string       `json:"deleted"`
	Pins    map[string]Pin `json:"pins"`
}

func fillClipDoc(d *clipDoc) {
	if d.Clips == nil {
		d.Clips = []Clip{}
	}
	if d.Deleted == nil {
		d.Deleted = []string{}
	}
	if d.Pins == nil {
		d.Pins = map[string]Pin{}
	}
}

// ClipOut is a clip as the dashboard sees it.
type ClipOut struct {
	Clip
	Pinned bool  `json:"pinned"`
	Exists *bool `json:"exists,omitempty"`
}

type ClipState struct {
	Version string    `json:"version"`
	Clips   []ClipOut `json:"clips"`
}

// ---------- notes ----------

type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	Updated   int64  `json:"updated"`
	By        string `json:"by"`
	ByID      string `json:"byId,omitempty"` // who edited last, e.g. "BAFDLNP"
	Created   int64  `json:"created"`
	CreatedBy string `json:"createdBy"`
}

type noteDoc struct {
	Device  string           `json:"device,omitempty"`
	ID      string           `json:"deviceId,omitempty"`
	Notes   map[string]Note  `json:"notes"`
	Deleted map[string]int64 `json:"deleted"`
}

func fillNoteDoc(d *noteDoc) {
	if d.Notes == nil {
		d.Notes = map[string]Note{}
	}
	if d.Deleted == nil {
		d.Deleted = map[string]int64{}
	}
}

type NoteState struct {
	Version string `json:"version"`
	Notes   []Note `json:"notes"`
	Note    *Note  `json:"note,omitempty"`
}

func version(v any) string {
	b, _ := json.Marshal(v)
	sum := md5.Sum(b)
	return hex.EncodeToString(sum[:])[:12]
}

func nowMs() int64 { return time.Now().UnixMilli() }

// mergeNotes picks the newest edit of every note and drops deleted ones.
func mergeNotes(docs []noteDoc) NoteState {
	best, deleted := map[string]Note{}, map[string]int64{}
	for _, d := range docs {
		for id, ts := range d.Deleted {
			if ts > deleted[id] {
				deleted[id] = ts
			}
		}
		for id, n := range d.Notes {
			if b, ok := best[id]; !ok || n.Updated > b.Updated {
				best[id] = n
			}
		}
	}
	out := []Note{}
	for id, n := range best {
		if n.Updated > deleted[id] {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated > out[j].Updated })
	return NoteState{Version: version(out), Notes: out}
}
