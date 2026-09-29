package update

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"0.1.2", "0.1.1", true}, {"0.1.10", "0.1.9", true}, {"1.0.0", "0.9.9", true}, {"v0.2.0", "0.1.5", true},
		{"0.1.1", "0.1.1", false}, {"0.1.0", "0.1.1", false},
		{"0.1.2", "dev", false}, {"dev", "0.1.1", false}, {"0.1", "0.0.9", false}, {"0.1.2-rc1", "0.1.1", false},
	} {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v; want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestChecksums(t *testing.T) {
	a, b := strings.Repeat("a", 64), strings.Repeat("B", 64)
	got := Checksums([]byte(a + "  SameDesk-1.0.0.dmg\n" + b + " *setup.exe\nnot a line\n"))
	if got["SameDesk-1.0.0.dmg"] != a || got["setup.exe"] != strings.ToLower(b) || len(got) != 2 {
		t.Errorf("Checksums = %v", got)
	}
}

func sign(t *testing.T, priv ed25519.PrivateKey, data []byte) []byte {
	return []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n")
}

func TestVerify(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	data := []byte("list")
	if err := Verify(data, sign(t, priv, data), pub); err != nil {
		t.Errorf("good signature: %v", err)
	}
	if Verify([]byte("lisT"), sign(t, priv, data), pub) == nil {
		t.Error("changed data accepted")
	}
	if Verify(data, sign(t, priv, data), other) == nil {
		t.Error("wrong key accepted")
	}
	if Verify(data, []byte("nonsense"), pub) == nil {
		t.Error("garbage signature accepted")
	}
}

// fake serves a release: its files, and SHA256SUMS.txt signed with priv.
func fake(t *testing.T, priv ed25519.PrivateKey, files map[string][]byte, signed bool) (*httptest.Server, release) {
	var list strings.Builder
	for name, body := range files {
		sum := sha256.Sum256(body)
		list.WriteString(hex.EncodeToString(sum[:]) + "  " + name + "\n")
	}
	serve := map[string][]byte{sums: []byte(list.String())}
	if signed {
		serve[sums+".sig"] = sign(t, priv, serve[sums])
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/")
		if b, ok := serve[name]; ok {
			_, _ = w.Write(b)
			return
		}
		if b, ok := files[name]; ok {
			_, _ = w.Write(b)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	rel := release{TagName: "v9.9.9", HTMLURL: "https://example.invalid/release"}
	for name := range serve {
		rel.Assets = append(rel.Assets, struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		}{name, srv.URL + "/" + name})
	}
	for name := range files {
		rel.Assets = append(rel.Assets, struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		}{name, srv.URL + "/" + name})
	}
	return srv, rel
}

func withKey(t *testing.T) ed25519.PrivateKey {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	old := publicKey
	publicKey = pub
	t.Cleanup(func() { publicKey = old })
	return priv
}

func TestDownload(t *testing.T) {
	name := assetName("9.9.9")
	if name == "" {
		t.Skip("no self-updating asset for this platform setup")
	}
	priv := withKey(t)
	body := []byte("new SameDesk")

	t.Run("signed and matching", func(t *testing.T) {
		_, rel := fake(t, priv, map[string][]byte{name: body}, true)
		u := New(Options{Current: "0.1.0", Dir: t.TempDir(), Enabled: true})
		u.download(rel, "9.9.9")
		st := u.Status()
		if st.State != "ready" || u.fileFor("9.9.9") == "" {
			t.Fatalf("state %q (%s), file %q", st.State, st.Error, u.file)
		}
		if got, _ := os.ReadFile(u.file); string(got) != string(body) {
			t.Errorf("downloaded %q", got)
		}
	})

	t.Run("file doesn't match its checksum", func(t *testing.T) {
		srv, rel := fake(t, priv, map[string][]byte{name: body}, true)
		// Serve something else under the same name.
		srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, name) {
				_, _ = w.Write([]byte("tampered"))
				return
			}
			http.NotFound(w, r)
		})
		u := New(Options{Current: "0.1.0", Dir: t.TempDir(), Enabled: true})
		u.download(rel, "9.9.9")
		if st := u.Status(); st.State == "ready" || u.fileFor("9.9.9") != "" {
			t.Fatalf("accepted a file that doesn't match: %+v", st)
		}
	})

	t.Run("signed with another key", func(t *testing.T) {
		_, stranger, _ := ed25519.GenerateKey(rand.Reader)
		_, rel := fake(t, stranger, map[string][]byte{name: body}, true)
		u := New(Options{Current: "0.1.0", Dir: t.TempDir(), Enabled: true})
		u.download(rel, "9.9.9")
		if st := u.Status(); st.State != "error" || u.fileFor("9.9.9") != "" {
			t.Fatalf("accepted a foreign signature: %+v", st)
		}
	})

	t.Run("unsigned release", func(t *testing.T) {
		_, rel := fake(t, priv, map[string][]byte{name: body}, false)
		u := New(Options{Current: "0.1.0", Dir: t.TempDir(), Enabled: true})
		u.download(rel, "9.9.9")
		if st := u.Status(); st.State != "manual" || u.fileFor("9.9.9") != "" {
			t.Fatalf("unsigned release should only be offered: %+v", st)
		}
	})
}

func TestSettingPersists(t *testing.T) {
	dir := t.TempDir()
	u := New(Options{Current: "0.1.0", Dir: dir})
	if !u.Status().Auto {
		t.Fatal("automatic updates should be on by default")
	}
	u.SetAuto(false)
	if New(Options{Current: "0.1.0", Dir: dir}).Status().Auto {
		t.Error("turning automatic updates off didn't stick")
	}
}
