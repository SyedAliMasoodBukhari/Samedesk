package hub

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/url"
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
		if err := h.eng.AddPeer(id, name); err != nil {
			return err
		}
		h.invalidateSync()
		h.writeJSON(w, 200, map[string]string{"id": id.String(), "name": name, "verify": verifyCode(h.eng.ID.String(), id.String())})
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
		if err := h.eng.AddPeer(id, name); err != nil {
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
		if err := h.eng.RemovePeer(id); err != nil {
			return err
		}
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
