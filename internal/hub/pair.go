package hub

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// PairPeer is a device we might pair with: seen nearby, or asking to connect.
type PairPeer struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	OS     string `json:"os,omitempty"`
	Addr   string `json:"addr,omitempty"`
	Verify string `json:"verify"`           // the check code both screens show
	Paired bool   `json:"paired,omitempty"` // already one of our devices
}

type PairState struct {
	Me        PairPeer   `json:"me"`
	Active    bool       `json:"active"`
	Available bool       `json:"available"` // local-network discovery works here
	Nearby    []PairPeer `json:"nearby"`
	Pending   []PairPeer `json:"pending"`
}

// verifyCode is a short code derived from both device IDs. Both screens show it;
// if they match, each side is talking to the device the user thinks it is.
func verifyCode(a, b string) string {
	if a > b {
		a, b = b, a
	}
	sum := sha256.Sum256([]byte(a + ":" + b))
	n := binary.BigEndian.Uint32(sum[:4]) % 1000000
	return fmt.Sprintf("%03d %03d", n/1000, n%1000)
}

func parseDeviceID(s string) (protocol.DeviceID, error) {
	s = strings.ToUpper(strings.Join(strings.Fields(s), ""))
	id, err := protocol.DeviceIDFromString(s)
	if err != nil {
		return id, badRequest("That doesn't look like a device code. Copy it again from the other device.")
	}
	return id, nil
}

func (h *Hub) pending() []PairPeer {
	var raw map[string]struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	}
	out := []PairPeer{}
	if err := h.st("/rest/cluster/pending/devices", &raw); err != nil {
		return out
	}
	me := h.eng.ID.String()
	kind := map[string]string{} // if the device is also announcing nearby, we know what it is
	for _, s := range h.beacon.Nearby() {
		kind[s.ID] = s.OS
	}
	for id, d := range raw {
		out = append(out, PairPeer{ID: id, Name: d.Name, OS: kind[id], Addr: d.Address, Verify: verifyCode(me, id)})
	}
	return out
}

func (h *Hub) pairState() PairState {
	me := h.eng.ID.String()
	st := PairState{Me: PairPeer{ID: me, Name: h.eng.Name(), OS: h.device}, Active: h.beacon.Active(),
		Available: h.beacon.Available(), Nearby: []PairPeer{}, Pending: h.pending()}
	for _, s := range h.beacon.Nearby() {
		p := PairPeer{ID: s.ID, Name: s.Name, OS: s.OS, Addr: s.Addr, Verify: verifyCode(me, s.ID)}
		if id, err := protocol.DeviceIDFromString(s.ID); err == nil {
			_, p.Paired = h.eng.Config().Device(id)
		}
		st.Nearby = append(st.Nearby, p)
	}
	return st
}

func (h *Hub) pairAPI(w http.ResponseWriter, r *http.Request) error {
	if !isLoopback(r) {
		return errForbidden // pairing is only ever done on the computer itself
	}
	var b struct{ ID string }
	if r.Method == http.MethodPost && r.ContentLength != 0 {
		if err := readJSON(r, &b); err != nil {
			return err
		}
	}
	switch p, m := r.URL.Path, r.Method; {
	case p == "/api/pair" && m == http.MethodGet:

	case p == "/api/pair/start" && m == http.MethodPost:
		h.beacon.Activate(10 * time.Minute)

	case p == "/api/pair/stop" && m == http.MethodPost:
		h.beacon.Deactivate()

	case p == "/api/pair/connect" && m == http.MethodPost:
		id, err := parseDeviceID(b.ID)
		if err != nil {
			return err
		}
		if id == h.eng.ID {
			return badRequest("That's this device's own code. Paste the code from the other device.")
		}
		name := ""
		for _, s := range h.beacon.Nearby() {
			if s.ID == id.String() {
				name = s.Name
			}
		}
		// Already one of our devices: say so, and don't let a Cancel remove it.
		if d, ok := h.eng.Config().Device(id); ok {
			h.writeJSON(w, 200, map[string]any{"id": id.String(), "name": d.Name, "existing": true})
			return nil
		}
		if err := h.eng.AddPeer(id, name, h.directAddrs(id)...); err != nil {
			return err
		}
		h.setAwaiting(id.String(), true)
		h.invalidateSync()
		h.writeJSON(w, 200, map[string]any{"id": id.String(), "name": name, "verify": verifyCode(h.eng.ID.String(), id.String())})
		return nil

	case p == "/api/pair/accept" && m == http.MethodPost:
		id, err := parseDeviceID(b.ID)
		if err != nil {
			return err
		}
		name := ""
		for _, pd := range h.pending() {
			if pd.ID == id.String() {
				name = pd.Name
			}
		}
		if err := h.eng.AddPeer(id, name, h.directAddrs(id)...); err != nil {
			return err
		}
		h.invalidateSync()

	case p == "/api/pair/decline" && m == http.MethodPost:
		id, err := parseDeviceID(b.ID)
		if err != nil {
			return err
		}
		h.dropPending(id)
		h.invalidateSync()

	case p == "/api/devices/remove" && m == http.MethodPost:
		id, err := parseDeviceID(b.ID)
		if err != nil {
			return err
		}
		if id == h.eng.ID {
			return badRequest("That's this computer. Remove other devices, not this one.")
		}
		if err := h.eng.RemovePeer(id); err != nil {
			return err
		}
		h.setAwaiting(id.String(), false)
		h.dropPending(id) // don't immediately ask again about a device we just removed
		h.invalidateSync()

	default:
		return errNotFound
	}
	h.writeJSON(w, 200, h.pairState())
	return nil
}

func (h *Hub) dropPending(id protocol.DeviceID) {
	req, _ := http.NewRequest(http.MethodDelete, strings.TrimRight(h.eng.APIURL, "/")+"/rest/cluster/pending/devices?device="+url.QueryEscape(id.String()), nil)
	req.Header.Set("X-API-Key", h.eng.APIKey)
	if resp, err := stClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

func (h *Hub) invalidateSync() {
	h.mu.Lock()
	h.sync = syncCache{}
	h.mu.Unlock()
}

// directAddrs is where a device on this network said it accepts connections, so
// Syncthing can connect at once rather than waiting to discover it.
func (h *Hub) directAddrs(id protocol.DeviceID) []string {
	s, ok := h.beacon.Lookup(id.String())
	if !ok || s.IP == "" || s.Port == 0 {
		return nil
	}
	hp := net.JoinHostPort(s.IP, strconv.Itoa(s.Port))
	return []string{"tcp://" + hp, "quic://" + hp}
}

// pairWatch brings a pairing request to the user's attention. When a device on
// this network that is pairing right now asks to connect, and no SameDesk page
// is showing, it opens the dashboard on the request: the other person is waiting
// and the check code has to be compared. Requests from anywhere else only show
// in the dashboard and the tray menu, so a stranger can't open windows here.
func (h *Hub) pairWatch() {
	for range time.Tick(2 * time.Second) {
		pending := h.pending()
		still := map[string]bool{}
		for _, p := range pending {
			still[p.ID] = true
		}
		for id := range h.prompted {
			if !still[id] { // answered or withdrawn: a new request may prompt again
				delete(h.prompted, id)
			}
		}
		for _, p := range pending {
			if h.prompted[p.ID] || !h.beacon.Seeking(p.ID) {
				continue
			}
			h.prompted[p.ID] = true
			if nowMs()-h.seen.Load() > 8000 {
				slog.Info("Opening SameDesk for a pairing request", "from", p.Name)
				OpenBrowser(h.URL() + "/#pair-request")
			}
		}
	}
}

// Devices this computer asked to pair with that haven't accepted yet. Until a
// device accepts, it refuses our connections, which looks just like being
// offline; remembering the request lets the dashboard say "Waiting for it to
// accept" instead. Kept in the settings folder, so it survives a restart.
func (h *Hub) awaitFile() string { return filepath.Join(h.local, "awaiting.json") }

func (h *Hub) loadAwaiting() map[string]int64 {
	m := map[string]int64{}
	if b, err := os.ReadFile(h.awaitFile()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func (h *Hub) setAwaiting(id string, on bool) {
	h.awaitMu.Lock()
	defer h.awaitMu.Unlock()
	m := h.loadAwaiting()
	if _, had := m[id]; had == on {
		return
	}
	if on {
		m[id] = nowMs()
	} else {
		delete(m, id)
	}
	b, _ := json.Marshal(m)
	_ = os.WriteFile(h.awaitFile(), b, 0o600)
}

func (h *Hub) awaiting(id string) bool {
	h.awaitMu.Lock()
	defer h.awaitMu.Unlock()
	_, ok := h.loadAwaiting()[id]
	return ok
}

func (h *Hub) accepted(id string) {
	if h.awaiting(id) {
		h.setAwaiting(id, false)
	}
}
