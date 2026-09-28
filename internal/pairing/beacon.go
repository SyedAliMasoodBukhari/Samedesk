// Package pairing finds other Shared Hub devices on the local network.
//
// While a user has "Add a device" open, the hub announces itself (name, kind,
// device ID) by UDP broadcast and listens for others doing the same. Nothing is
// announced otherwise. The announcement is only a hint: trust comes from
// Syncthing's device IDs (TLS certificate fingerprints) and the matching check
// code both people see before accepting.
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
	appName = "shared-hub"
	every   = 1500 * time.Millisecond
	stale   = 6 * time.Second
)

// Announce is what a device says about itself.
type Announce struct {
	App  string `json:"app"`
	V    int    `json:"v"`
	ID   string `json:"id"`   // full Syncthing device ID
	Name string `json:"name"` // e.g. "Ali's MacBook Air"
	OS   string `json:"os"`   // "Mac", "Windows" or "Linux"
}

// Seen is a device heard on the network.
type Seen struct {
	Announce
	Addr string    `json:"addr"`
	At   time.Time `json:"-"`
}

type Beacon struct {
	self  Announce
	conn  net.PacketConn
	mu    sync.Mutex
	until time.Time
	seen  map[string]Seen
}

// Start listens on the pairing port. If the port can't be opened the beacon
// stays silent and pairing falls back to device codes.
func Start(id, name, os string) *Beacon {
	b := &Beacon{self: Announce{App: appName, V: 1, ID: id, Name: name, OS: os}, seen: map[string]Seen{}}
	conn, err := listen(Port)
	if err != nil {
		return b
	}
	b.conn = conn
	go b.receive()
	go b.announce()
	return b
}

// Activate announces this device for d (while the pairing screen is open).
func (b *Beacon) Activate(d time.Duration) {
	b.mu.Lock()
	b.until = time.Now().Add(d)
	b.mu.Unlock()
}

func (b *Beacon) Deactivate() {
	b.mu.Lock()
	b.until = time.Time{}
	b.seen = map[string]Seen{}
	b.mu.Unlock()
}

func (b *Beacon) Active() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return time.Now().Before(b.until)
}

// Available reports whether local discovery works on this machine.
func (b *Beacon) Available() bool { return b.conn != nil }

// Nearby lists devices currently announcing, newest name first.
func (b *Beacon) Nearby() []Seen {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := []Seen{}
	for id, s := range b.seen {
		if time.Since(s.At) > stale {
			delete(b.seen, id)
			continue
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (b *Beacon) announce() {
	msg, _ := json.Marshal(b.self)
	targets := []*net.UDPAddr{{IP: net.IPv4bcast, Port: Port}, {IP: net.IPv4(127, 0, 0, 1), Port: Port}}
	for range time.Tick(every) {
		if !b.Active() {
			continue
		}
		for _, t := range append(targets, directedBroadcasts()...) {
			_, _ = b.conn.WriteTo(msg, t)
		}
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
		if !b.Active() {
			continue // only collect while we're looking
		}
		host, _, _ := net.SplitHostPort(from.String())
		b.mu.Lock()
		b.seen[a.ID] = Seen{Announce: a, Addr: host, At: time.Now()}
		b.mu.Unlock()
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
