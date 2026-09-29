package hub

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/syncthing/syncthing/lib/protocol"
)

// Sync status comes from the embedded Syncthing's REST API on loopback.

type Peer struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Label     string  `json:"label"`
	Connected bool    `json:"connected"`
	Shared    bool    `json:"shared"` // the other side accepted and shares the folder
	Need      int     `json:"need"`
	LastSeen  *string `json:"lastSeen"`
}

type SyncStatus struct {
	OK        bool       `json:"ok"`
	Reason    string     `json:"reason,omitempty"`
	State     string     `json:"state,omitempty"`
	Receiving int        `json:"receiving"`
	Errors    int        `json:"errors"`
	Peers     []Peer     `json:"peers"`
	Pending   []PairPeer `json:"pending"` // devices asking to connect
	Paused    bool       `json:"paused"`
}

type syncCache struct {
	at   time.Time
	data SyncStatus
}

var stClient = &http.Client{Timeout: 2 * time.Second}

func (h *Hub) st(path string, out any) error {
	req, _ := http.NewRequest(http.MethodGet, strings.TrimRight(h.eng.APIURL, "/")+path, nil)
	req.Header.Set("X-API-Key", h.eng.APIKey)
	resp, err := stClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("syncthing %s: %s", path, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// rescan tells Syncthing exactly what changed, so edits go out at once instead of
// waiting for the file watcher (or the hourly rescan where no watcher is available).
func (h *Hub) rescan(subs ...string) {
	go func() {
		for _, sub := range subs {
			q := "/rest/db/scan?folder=" + url.QueryEscape(h.eng.FolderID)
			if sub != "" {
				q += "&sub=" + url.QueryEscape(sub)
			}
			req, _ := http.NewRequest(http.MethodPost, strings.TrimRight(h.eng.APIURL, "/")+q, nil)
			req.Header.Set("X-API-Key", h.eng.APIKey)
			if resp, err := stClient.Do(req); err == nil {
				resp.Body.Close()
			}
		}
	}()
}

func (h *Hub) syncStatus() SyncStatus {
	h.mu.Lock()
	if time.Since(h.sync.at) < 2*time.Second {
		d := h.sync.data
		h.mu.Unlock()
		return d
	}
	// Each device's hub files say what it is ("Windows", "Linux"…) and what it's
	// called right now, keyed by short device ID.
	who := map[string]identity{}
	for short, d := range h.notes.named() {
		who[short] = identity{d.Device, d.Name}
	}
	for short, d := range h.clips.named() {
		i := who[short]
		who[short] = identity{firstOf(d.Device, i.kind), firstOf(d.Name, i.name)}
	}
	h.mu.Unlock()

	data, err := h.fetchStatus(who)
	if err != nil {
		data = SyncStatus{Reason: "The sync engine isn't responding"}
	}
	h.mu.Lock()
	h.sync = syncCache{at: time.Now(), data: data}
	h.mu.Unlock()
	return data
}

type identity struct{ kind, name string }

func firstOf(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

func (h *Hub) fetchStatus(who map[string]identity) (SyncStatus, error) {
	fid := url.QueryEscape(h.eng.FolderID)
	var folder struct {
		Devices []struct {
			DeviceID string `json:"deviceID"`
		} `json:"devices"`
		Paused bool `json:"paused"`
	}
	if err := h.st("/rest/config/folders/"+fid, &folder); err != nil {
		return SyncStatus{}, err
	}
	var devices []struct{ DeviceID, Name string }
	var conns struct {
		Connections map[string]struct{ Connected bool } `json:"connections"`
	}
	var stats map[string]struct{ LastSeen string }
	var local struct {
		State      string `json:"state"`
		NeedFiles  int    `json:"needFiles"`
		Errors     int    `json:"errors"`
		PullErrors int    `json:"pullErrors"`
	}
	for p, v := range map[string]any{"/rest/config/devices": &devices, "/rest/system/connections": &conns,
		"/rest/stats/device": &stats, "/rest/db/status?folder=" + fid: &local} {
		if err := h.st(p, v); err != nil {
			return SyncStatus{}, err
		}
	}
	names := map[string]string{}
	for _, d := range devices {
		names[d.DeviceID] = d.Name
	}
	myID := h.eng.ID.String()
	var peerIDs []string
	for _, d := range folder.Devices {
		if d.DeviceID != myID {
			peerIDs = append(peerIDs, d.DeviceID)
		}
	}
	peers := []Peer{}
	for _, pid := range peerIDs {
		var comp struct {
			NeedItems, NeedDeletes int
			RemoteState            string `json:"remoteState"`
		}
		_ = h.st(fmt.Sprintf("/rest/db/completion?folder=%s&device=%s", fid, url.QueryEscape(pid)), &comp)
		// The name a device gives itself wins over the one it had when it was paired.
		var me identity
		if id, err := protocol.DeviceIDFromString(pid); err == nil {
			me = who[id.Short().String()]
		}
		name := firstOf(me.name, names[pid], me.kind, "Device "+pid[:7])
		label := firstOf(me.kind, name) // "Windows", "Mac" or "Linux" once its files are here
		p := Peer{ID: pid, Name: name, Label: label, Connected: conns.Connections[pid].Connected, Need: comp.NeedItems + comp.NeedDeletes,
			Shared: comp.RemoteState == "valid"}
		if seen := stats[pid].LastSeen; seen != "" && !strings.HasPrefix(seen, "1970") {
			p.LastSeen = &seen
		}
		peers = append(peers, p)
	}
	// Two devices of the same kind (say, two Macs) are told apart by their own names.
	count := map[string]int{h.device: 1}
	for _, p := range peers {
		count[p.Label]++
	}
	for i := range peers {
		if count[peers[i].Label] > 1 {
			peers[i].Label = peers[i].Name
		}
	}
	return SyncStatus{OK: true, State: local.State, Receiving: local.NeedFiles, Errors: local.Errors + local.PullErrors,
		Peers: peers, Pending: h.pending(), Paused: folder.Paused}, nil
}
