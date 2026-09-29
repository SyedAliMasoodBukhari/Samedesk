package tray

import (
	"testing"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/hub"
)

func TestDescribe(t *testing.T) {
	pc := hub.Peer{Name: "Office PC", Connected: true, Shared: true}
	offline := hub.Peer{Name: "Office PC"}
	laptop := hub.Peer{Name: "Linux laptop", Connected: true, Shared: true}
	for _, c := range []struct {
		s          hub.SyncStatus
		line, peer string
	}{
		{hub.SyncStatus{}, "The sync engine isn't responding", ""},
		{hub.SyncStatus{OK: true}, "No devices yet", ""},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{pc}}, "In sync with Office PC", "Office PC"},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{offline}}, "Office PC is offline", "Office PC"},
		{hub.SyncStatus{OK: true, Paused: true, Peers: []hub.Peer{pc}}, "Sync is paused", "Office PC"},
		{hub.SyncStatus{OK: true, Receiving: 3, Peers: []hub.Peer{pc}}, "Syncing with Office PC…", "Office PC"},
		{hub.SyncStatus{OK: true, Errors: 2, Peers: []hub.Peer{pc}}, "2 files couldn't sync", "Office PC"},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{{Name: "Office PC", Connected: true}}}, "Waiting for Office PC to accept", "Office PC"},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{pc, laptop}}, "In sync with 2 devices", ""},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{offline, laptop}}, "In sync with 1 of 2 devices", ""},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{offline, {Name: "Linux laptop"}}}, "Your devices are offline", ""},
	} {
		line, peer := describe(c.s)
		if line != c.line || peer != c.peer {
			t.Errorf("describe(%+v) = %q, %q; want %q, %q", c.s, line, peer, c.line, c.peer)
		}
	}
}

func TestLatestTitle(t *testing.T) {
	yes, no := true, false
	clip := func(c hub.Clip, exists *bool) hub.Incoming {
		return hub.Incoming{ClipOut: hub.ClipOut{Clip: c, Exists: exists}, Sender: "Office PC"}
	}
	for _, c := range []struct {
		in      hub.Incoming
		title   string
		enabled bool
	}{
		{clip(hub.Clip{Kind: "text", Text: "\n  Launch copy, final\nsecond line"}, nil), "Copy from Office PC: “Launch copy, final”", true},
		{clip(hub.Clip{Kind: "text", Text: "hunter2", Sensitive: true}, nil), "Copy private text from Office PC", true},
		{clip(hub.Clip{Kind: "file", Name: "shot.png"}, &yes), "Copy image from Office PC: “shot.png”", true},
		{clip(hub.Clip{Kind: "file", Name: "report.pdf"}, &yes), "Show file from Office PC: “report.pdf”", true},
		{clip(hub.Clip{Kind: "file", Name: "report.pdf"}, &no), "Receiving “report.pdf” from Office PC…", false},
	} {
		title, enabled := latestTitle(c.in)
		if title != c.title || enabled != c.enabled {
			t.Errorf("latestTitle(%+v) = %q, %v; want %q, %v", c.in.Clip, title, enabled, c.title, c.enabled)
		}
	}
}

func TestShort(t *testing.T) {
	for in, want := range map[string]string{
		"short":                                "short",
		"The quick brown fox jumps over it":    "The quick brown fox jumps…",
		"Supercalifragilisticexpialidocious!!": "Supercalifragilisticexpiali…",
		"  spaced\t out   text ":               "spaced out text",
		"ابجد هوز حطي كلمن سعفص قرشت ثخذ ضظغ": "ابجد هوز حطي كلمن سعفص…",
	} {
		if got := short(in, 27); got != want {
			t.Errorf("short(%q) = %q; want %q", in, got, want)
		}
	}
}
