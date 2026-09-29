// Package pairing finds other SameDesk devices on the local network.
//
// A device with "Add a device" open asks who is there, by UDP broadcast, every
// second and a half. Every SameDesk that hears the question answers with its
// name, kind, device ID and sync port, so the other computer doesn't need to be
// doing anything. Nothing is sent while nobody is pairing. The answers are only
// hints: trust comes from Syncthing's device IDs (TLS certificate fingerprints)
// and the check code both people compare before accepting.
package pairing

import (
	"encoding/json"
	"net"
	"sort"
	"sync"
	"time"
)

const (
	Port    = 21099
	appName = "samedesk"
	every   = 1500 * time.Millisecond
	stale   = 6 * time.Second
)

// Announce is what a device says about itself.
type Announce struct {
	App  string `json:"app"`
	V    int    `json:"v"`
	ID   string `json:"id"`             // full Syncthing device ID
	Name string `json:"name"`           // e.g. "Ali's MacBook Air"
	OS   string `json:"os"`             // "Mac", "Windows" or "Linux"
	Port int    `json:"port,omitempty"` // its sync port, so pairing can connect straight away
	Seek bool   `json:"seek,omitempty"` // "who's there?": sent while "Add a device" is open
}

// Seen is a device heard on the network.
type Seen struct {
	Announce
	Addr string    `json:"addr"` // shown to people; blank for this same computer
	IP   string    `json:"-"`    // where to connect
	At   time.Time `json:"-"`
}

type Beacon struct {
	self    Announce
	conn    net.PacketConn
	mu      sync.Mutex
	until   time.Time
	heard   map[string]Seen
	asked   map[string]time.Time // when each device last asked who's there
	replied time.Time
}

// Start listens on the pairing port. If the port can't be opened the beacon
// stays silent and pairing falls back to device codes.
func Start(id, name, os string) *Beacon {
	b := &Beacon{self: Announce{App: appName, V: 2, ID: id, Name: name, OS: os}, heard: map[string]Seen{}, asked: map[string]time.Time{}}
	conn, err := listen(Port)
	if err != nil {
		return b
	}
	b.conn = conn
	go b.receive()
	go b.ask()
	return b
}

// SetPort tells others which port to connect to.
func (b *Beacon) SetPort(p int) {
	b.mu.Lock()
	b.self.Port = p
	b.mu.Unlock()
}

// Activate asks who is around for d (while the pairing screen is open).
func (b *Beacon) Activate(d time.Duration) {
	b.mu.Lock()
	b.until = time.Now().Add(d)
	b.mu.Unlock()
	go b.send(true) // don't wait for the first tick
}

func (b *Beacon) Deactivate() {
	b.mu.Lock()
	b.until = time.Time{}
	b.mu.Unlock()
}

func (b *Beacon) Active() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Now().Before(b.until)
}

// Available reports whether local discovery works on this machine.
func (b *Beacon) Available() bool { return b.conn != nil }

// Nearby lists the devices that answered, while this device is looking.
func (b *Beacon) Nearby() []Seen {
	if !b.Active() {
		return []Seen{}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []Seen{}
	for id, s := range b.heard {
		if time.Since(s.At) > stale {
			delete(b.heard, id)
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Lookup returns what a device last said, if it was heard recently.
func (b *Beacon) Lookup(id string) (Seen, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.heard[id]
	return s, ok && time.Since(s.At) <= stale
}

// Seeking reports whether a device on this network is pairing right now.
func (b *Beacon) Seeking(id string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	at, ok := b.asked[id]
	return ok && time.Since(at) <= stale
}

func (b *Beacon) ask() {
	for range time.Tick(every) {
		if b.Active() {
			b.send(true)
		}
	}
}

// send broadcasts this device's announcement, as a question or as an answer.
func (b *Beacon) send(seek bool) {
	b.mu.Lock()
	a := b.self
	b.mu.Unlock()
	a.Seek = seek
	msg, _ := json.Marshal(a)
	// Answers are broadcast too: two copies on one computer share the port, and a
	// direct reply could reach the wrong one.
	targets := []*net.UDPAddr{{IP: net.IPv4bcast, Port: Port}, {IP: net.IPv4(127, 0, 0, 1), Port: Port}}
	for _, t := range append(targets, directedBroadcasts()...) {
		_, _ = b.conn.WriteTo(msg, t)
	}
}

func (b *Beacon) receive() {
	buf := make([]byte, 2048)
	for {
		n, from, err := b.conn.ReadFrom(buf)
		if err != nil {
			return
		}
		var a Announce
		if json.Unmarshal(buf[:n], &a) != nil || a.App != appName || a.ID == "" || a.ID == b.self.ID {
			continue
		}
		if a.V < 2 {
			a.Seek = true // version 1 only announced while pairing
		}
		host, _, _ := net.SplitHostPort(from.String())
		shown := host
		if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
			shown = "" // same machine; nothing useful to show
		}
		b.mu.Lock()
		b.heard[a.ID] = Seen{Announce: a, Addr: shown, IP: host, At: time.Now()}
		if a.Seek {
			b.asked[a.ID] = time.Now()
		}
		answer := a.Seek && time.Since(b.replied) > time.Second
		if answer {
			b.replied = time.Now()
		}
		b.mu.Unlock()
		if answer {
			go b.send(false)
		}
	}
}

// directedBroadcasts returns each interface's subnet broadcast address; some
// networks drop 255.255.255.255 but pass these.
func directedBroadcasts() []*net.UDPAddr {
	var out []*net.UDPAddr
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagBroadcast == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			ip, mask := ipn.IP.To4(), ipn.Mask
			if len(mask) == 16 {
				mask = mask[12:]
			}
			bc := make(net.IP, 4)
			for i := range bc {
				bc[i] = ip[i] | ^mask[i]
			}
			out = append(out, &net.UDPAddr{IP: bc, Port: Port})
		}
	}
	return out
}
