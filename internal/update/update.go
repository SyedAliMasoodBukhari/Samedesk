// Package update keeps an installed SameDesk up to date from GitHub releases.
//
// It looks at the latest published release a couple of minutes after starting and
// then every six hours. A newer version is downloaded only if the release's
// SHA256SUMS.txt carries a valid signature from the SameDesk release key (built
// into the app) and the download matches its checksum. Then, once sync is idle,
// it installs the new version and restarts: the .app on macOS, the installer on
// Windows, the AppImage on Linux. Copies installed by a package manager (.deb,
// .rpm) can't replace themselves, so for them it only says an update is out.
package update

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	repo      = "SyedAliMasoodBukhari/Samedesk"
	firstLook = 2 * time.Minute
	every     = 6 * time.Hour
	sums      = "SHA256SUMS.txt"
)

// Status is what the dashboard and the tray show.
type Status struct {
	Current   string `json:"current"`
	Enabled   bool   `json:"enabled"` // updates are on for this copy at all
	Latest    string `json:"latest,omitempty"`
	Available bool   `json:"available"`         // a newer version is out
	Auto      bool   `json:"auto"`              // install updates by itself
	CanAuto   bool   `json:"canAuto"`           // this copy can replace itself
	State     string `json:"state"`             // idle, checking, downloading, ready, installing, manual, error
	Error     string `json:"error,omitempty"`   // what went wrong, in plain words
	Page      string `json:"page,omitempty"`    // the release page
	Checked   int64  `json:"checked,omitempty"` // unix ms of the last check
}

type Options struct {
	Current string          // this build's version, e.g. "0.1.2"
	Dir     string          // where downloads and the setting live
	Enabled bool            // false for development builds: never touch them
	Idle    func() bool     // whether now is a good moment to restart
	Args    func() []string // flags for the restarted copy
	Quit    func()          // shuts this copy down cleanly
}

type Updater struct {
	o      Options
	client *http.Client
	mu     sync.Mutex
	st     Status
	file   string // the verified download, ready to install
	want   bool   // someone asked to update now: install as soon as it's downloaded
	kick   chan struct{}
}

type release struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Assets  []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

func New(o Options) *Updater {
	u := &Updater{o: o, client: &http.Client{Timeout: 10 * time.Minute}, kick: make(chan struct{}, 1)}
	u.st = Status{Current: o.Current, Enabled: o.Enabled, Auto: u.loadAuto(), CanAuto: o.Enabled && selfUpdatable(), State: "idle"}
	return u
}

// Run checks now and then, and installs when there is something to install.
// It returns only if updates are off for this build.
func (u *Updater) Run() {
	if !u.o.Enabled {
		return
	}
	cleanup(u.o.Dir)
	timer := time.NewTimer(firstLook)
	idle := time.NewTicker(time.Minute)
	for {
		select {
		case <-timer.C:
			u.check(true)
			timer.Reset(every)
		case <-u.kick:
			u.check(false)
		case <-idle.C:
		}
		u.mu.Lock()
		ready, asked, auto := u.st.State == "ready", u.want, u.st.Auto
		u.mu.Unlock()
		if ready && (asked || (auto && (u.o.Idle == nil || u.o.Idle()))) {
			u.Install()
		}
	}
}

func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.st
}

// Check looks for a new version now.
func (u *Updater) Check() {
	select {
	case u.kick <- struct{}{}:
	default:
	}
}

func (u *Updater) SetAuto(on bool) {
	u.mu.Lock()
	u.st.Auto = on
	u.mu.Unlock()
	_ = os.MkdirAll(u.o.Dir, 0o700)
	b, _ := json.Marshal(map[string]bool{"auto": on})
	_ = os.WriteFile(filepath.Join(u.o.Dir, "update.json"), b, 0o600)
	if on {
		u.Check()
	}
}

func (u *Updater) loadAuto() bool {
	var s struct{ Auto *bool }
	if b, err := os.ReadFile(filepath.Join(u.o.Dir, "update.json")); err == nil && json.Unmarshal(b, &s) == nil && s.Auto != nil {
		return *s.Auto
	}
	return true
}

func (u *Updater) set(f func(*Status)) {
	u.mu.Lock()
	f(&u.st)
	u.mu.Unlock()
}

// check looks at the latest release and, if it's newer, downloads it when this
// copy can install it and should (automatic updates on, or asked for).
func (u *Updater) check(scheduled bool) {
	u.mu.Lock()
	if u.st.State == "checking" || u.st.State == "downloading" || u.st.State == "installing" {
		u.mu.Unlock()
		return
	}
	u.st.State, u.st.Error = "checking", ""
	auto, canAuto, asked := u.st.Auto, u.st.CanAuto, u.want
	u.mu.Unlock()

	rel, err := u.latest()
	if err != nil {
		slog.Warn("Update check failed", "error", err)
		u.set(func(s *Status) {
			s.State, s.Error, s.Checked = "error", "Couldn't reach GitHub to check for updates.", nowMs()
		})
		return
	}
	v := strings.TrimPrefix(rel.TagName, "v")
	newer := Newer(v, u.o.Current)
	u.set(func(s *Status) { s.Latest, s.Available, s.Page, s.Checked = v, newer, rel.HTMLURL, nowMs() })
	if !newer {
		u.set(func(s *Status) { s.State = "idle" })
		return
	}
	if !canAuto {
		u.set(func(s *Status) { s.State = "manual" })
		return
	}
	if u.fileFor(v) != "" {
		u.set(func(s *Status) { s.State = "ready" })
		return
	}
	if scheduled && !auto && !asked {
		u.set(func(s *Status) { s.State = "idle" }) // shown as available; downloaded when asked
		return
	}
	u.download(rel, v)
}

// Install puts the downloaded version in place and restarts into it.
func (u *Updater) Install() {
	u.mu.Lock()
	file, v, state := u.file, u.st.Latest, u.st.State
	u.mu.Unlock()
	if state == "manual" || state == "installing" {
		return
	}
	if file == "" || u.fileFor(v) == "" {
		u.mu.Lock()
		u.want = true // not downloaded yet: fetch it, then install straight away
		u.mu.Unlock()
		u.Check()
		return
	}
	u.set(func(s *Status) { s.State = "installing" })
	u.mu.Lock()
	u.want = false
	u.mu.Unlock()
	slog.Info("Installing update", "version", v)
	args := []string{"-open=false"}
	if u.o.Args != nil {
		args = u.o.Args()
	}
	if err := apply(file, append(args, "-restarted")); err != nil {
		slog.Error("Update failed", "error", err)
		u.set(func(s *Status) {
			s.State, s.Error = "error", "The update couldn't be installed. SameDesk carries on with this version."
		})
		return
	}
	if u.o.Quit != nil {
		u.o.Quit()
	}
}

func (u *Updater) latest() (release, error) {
	var rel release
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "SameDesk/"+u.o.Current)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	resp, err := u.client.Do(req.WithContext(ctx))
	if err != nil {
		return rel, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return rel, fmt.Errorf("github: %s", resp.Status)
	}
	return rel, json.NewDecoder(resp.Body).Decode(&rel)
}

func (u *Updater) download(rel release, v string) {
	u.set(func(s *Status) { s.State = "downloading" })
	fail := func(msg string, err error) {
		slog.Warn("Update download failed", "error", err)
		u.set(func(s *Status) { s.State, s.Error = "error", msg })
	}
	name := assetName(v)
	urls := map[string]string{}
	for _, a := range rel.Assets {
		urls[a.Name] = a.URL
	}
	if name == "" || urls[name] == "" {
		u.set(func(s *Status) { s.State = "manual" })
		return
	}
	if urls[sums+".sig"] == "" {
		slog.Warn("Update not signed; not installing it automatically", "version", v)
		u.set(func(s *Status) {
			s.State, s.Error = "manual", "This release isn't signed, so it can't be installed automatically."
		})
		return
	}
	list, err := u.get(urls[sums], 1<<20)
	if err != nil {
		fail("Couldn't download the update.", err)
		return
	}
	sig, err := u.get(urls[sums+".sig"], 4096)
	if err != nil {
		fail("Couldn't download the update.", err)
		return
	}
	if err := Verify(list, sig, publicKey); err != nil {
		fail("The update's signature didn't check out, so it wasn't installed.", err)
		return
	}
	want, ok := Checksums(list)[name]
	if !ok {
		fail("The update's checksum list doesn't include this platform.", errors.New(name+" missing from "+sums))
		return
	}
	dir := filepath.Join(u.o.Dir, "update")
	_ = os.RemoveAll(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fail("Couldn't save the update.", err)
		return
	}
	path := filepath.Join(dir, name)
	if err := u.save(urls[name], path, want); err != nil {
		_ = os.RemoveAll(dir)
		fail("Couldn't download the update.", err)
		return
	}
	u.mu.Lock()
	u.file = path
	u.st.State = "ready"
	u.mu.Unlock()
	slog.Info("Update downloaded and verified", "version", v)
}

// fileFor is the verified download for version v, if there is one.
func (u *Updater) fileFor(v string) string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.file != "" && filepath.Base(u.file) == assetName(v) {
		if _, err := os.Stat(u.file); err == nil {
			return u.file
		}
	}
	return ""
}

func (u *Updater) get(url string, limit int64) ([]byte, error) {
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

// save downloads url to path, checking its SHA-256 on the way.
func (u *Updater) save(url, path, want string) error {
	resp, err := u.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s: %s", url, resp.Status)
	}
	tmp := path + ".part"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o700)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, 512<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		_ = os.Remove(tmp)
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, want)
	}
	return os.Rename(tmp, path)
}

// Verify checks a base64 ed25519 signature over data.
func Verify(data, sig []byte, key ed25519.PublicKey) error {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(raw) != ed25519.SignatureSize {
		return errors.New("malformed signature")
	}
	if !ed25519.Verify(key, data, raw) {
		return errors.New("bad signature")
	}
	return nil
}

// Checksums reads a sha256sum listing: "<hex>  <name>" per line.
func Checksums(list []byte) map[string]string {
	out := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(list))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && len(f[0]) == 64 {
			out[strings.TrimPrefix(f[1], "*")] = strings.ToLower(f[0])
		}
	}
	return out
}

// Newer reports whether version a is later than b ("0.1.10" > "0.1.9"). Anything
// that isn't a plain release number, like "dev", is never newer and never updated.
func Newer(a, b string) bool {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka || !okb {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

// Parse reads a plain release number such as "0.1.2".
func Parse(v string) ([3]int, bool) { return parse(v) }

func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

// cleanup removes what a previous update left behind.
func cleanup(dir string) {
	_ = os.RemoveAll(filepath.Join(dir, "update"))
	cleanupOld()
}

func nowMs() int64 { return time.Now().UnixMilli() }
