package tray

import (
	"bytes"
	"image/png"
	"testing"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/hub"
)

func TestDescribe(t *testing.T) {
	on := hub.Peer{Name: "Office PC", Connected: true, Shared: true}
	off := hub.Peer{Name: "Office PC"}
	for _, c := range []struct {
		s    hub.SyncStatus
		want state
	}{
		{hub.SyncStatus{}, state{"Not running", red}},
		{hub.SyncStatus{OK: true}, state{"No devices yet", grey}},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{on}}, state{"Up to date", green}},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{off}}, state{"Offline", grey}},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{off, on}}, state{"Up to date", green}},
		{hub.SyncStatus{OK: true, Paused: true, Peers: []hub.Peer{on}}, state{"Paused", grey}},
		{hub.SyncStatus{OK: true, Receiving: 3, Peers: []hub.Peer{on}}, state{"Syncing…", blue}},
		{hub.SyncStatus{OK: true, Peers: []hub.Peer{{Connected: true, Need: 2}}}, state{"Syncing…", blue}},
		{hub.SyncStatus{OK: true, Errors: 2, Peers: []hub.Peer{on}}, state{"Some files couldn't sync", red}},
	} {
		if got := describe(c.s); got != c.want {
			t.Errorf("describe(%+v) = %+v; want %+v", c.s, got, c.want)
		}
	}
}

func TestDot(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(dot(green)))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, a := img.At(16, 16).RGBA(); a != 0xffff {
		t.Errorf("centre alpha = %x; want opaque", a)
	}
	if _, _, _, a := img.At(1, 1).RGBA(); a != 0 {
		t.Errorf("corner alpha = %x; want clear", a)
	}
	ico := pngToICO(dot(blue))
	if !bytes.Equal(ico[:6], []byte{0, 0, 1, 0, 1, 0}) || !bytes.HasPrefix(ico[22:], []byte("\x89PNG")) {
		t.Errorf("bad ico header % x", ico[:24])
	}
}
