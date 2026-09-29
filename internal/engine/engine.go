// Package engine runs Syncthing inside SameDesk.
//
// Syncthing is embedded as a library (github.com/syncthing/syncthing/lib/syncthing),
// the same way the syncthing binary starts itself. It gets its own home directory,
// so it never touches a Syncthing the user may already run, and it listens on free
// ports chosen at first start.
package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/syncthing/syncthing/lib/config"
	"github.com/syncthing/syncthing/lib/events"
	"github.com/syncthing/syncthing/lib/locations"
	"github.com/syncthing/syncthing/lib/protocol"
	"github.com/syncthing/syncthing/lib/svcutil"
	"github.com/syncthing/syncthing/lib/syncthing"
	"github.com/thejerf/suture/v4"
)

// Options configure the embedded engine.
type Options struct {
	HomeDir    string // Syncthing's own config, keys and database
	FolderPath string // the shared folder on disk
	FolderID   string // Syncthing folder ID, the same on every device
	FolderName string // label shown in Syncthing
	DeviceName string // how this computer appears to the others, e.g. "Ali's MacBook Air"
	ForceName  bool   // apply DeviceName even if the device was already named
}

// Engine is a running, embedded Syncthing.
type Engine struct {
	ID       protocol.DeviceID
	FolderID string
	APIURL   string // Syncthing's REST API, loopback only
	APIKey   string

	app    *syncthing.App
	cfg    config.Wrapper
	cancel context.CancelFunc
}

// deleteRetention matches Syncthing's default for deleted-file records.
const deleteRetention = 10920 * time.Hour

// Start brings Syncthing up and returns once its API is ready.
func Start(o Options) (*Engine, error) {
	if err := os.MkdirAll(o.HomeDir, 0o700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(o.FolderPath, 0o755); err != nil {
		return nil, err
	}
	for _, base := range []locations.BaseDirEnum{locations.ConfigBaseDir, locations.DataBaseDir} {
		if err := locations.SetBaseDir(base, o.HomeDir); err != nil {
			return nil, err
		}
	}

	cert, err := syncthing.LoadOrGenerateCertificate(locations.Get(locations.CertFile), locations.Get(locations.KeyFile))
	if err != nil {
		return nil, fmt.Errorf("certificate: %w", err)
	}
	myID := protocol.NewDeviceID(cert.Certificate[0])

	// The event logger and config service run for the whole lifetime of the app.
	ctx, cancel := context.WithCancel(context.Background())
	early := suture.New("early", svcutil.SpecWithDebugLogger())
	early.ServeBackground(ctx)
	evLogger := events.NewLogger()
	early.Add(evLogger)

	cfg, err := syncthing.LoadConfigAtStartup(locations.Get(locations.ConfigFile), cert, evLogger, false, false)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("config: %w", err)
	}
	early.Add(cfg)
	_ = os.Remove(locations.Get(locations.ConfigFile) + ".v0") // copy Syncthing archives when it writes a brand-new config

	if err := prepare(cfg, myID, o); err != nil {
		cancel()
		return nil, fmt.Errorf("prepare config: %w", err)
	}

	sdb, err := syncthing.OpenDatabase(locations.Get(locations.Database), deleteRetention)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("database: %w", err)
	}

	app, err := syncthing.New(cfg, sdb, evLogger, cert, syncthing.Options{NoUpgrade: true})
	if err != nil {
		cancel()
		return nil, err
	}
	if err := app.Start(); err != nil {
		cancel()
		return nil, err
	}

	gui := cfg.GUI()
	return &Engine{ID: myID, FolderID: o.FolderID, APIURL: gui.URL(), APIKey: gui.APIKey, app: app, cfg: cfg, cancel: cancel}, nil
}

// prepare makes the config right for an app nobody configures by hand:
// a private API, no Syncthing pop-ups or prompts, and the shared folder in place.
func prepare(cfg config.Wrapper, myID protocol.DeviceID, o Options) error {
	marker := filepath.Join(o.HomeDir, "custom-name")
	_, err := os.Stat(marker)
	custom := err == nil
	if o.ForceName {
		_ = os.WriteFile(marker, []byte(o.DeviceName+"\n"), 0o600)
	}
	w, err := cfg.Modify(func(c *config.Configuration) {
		if c.GUI.APIKey == "" {
			c.GUI.APIKey = randomKey()
		}
		c.Options.StartBrowser = false     // SameDesk is the UI
		c.Options.URAccepted = -1          // don't ask about usage reporting
		c.Options.AutoUpgradeIntervalH = 0 // updates ship with SameDesk itself
		c.Options.CREnabled = false        // no crash reports to third parties by default

		f, _, ok := c.Folder(o.FolderID)
		if !ok {
			f = c.Defaults.Folder.Copy()
			f.ID = o.FolderID
			f.Devices = []config.FolderDeviceConfiguration{{DeviceID: myID}}
		}
		f.Label = o.FolderName
		f.Path = o.FolderPath
		f.Type = config.FolderTypeSendReceive
		f.FSWatcherEnabled = true
		f.FSWatcherDelayS = 1 // clipboard and notes should arrive in seconds, not ten
		c.SetFolder(f)

		// Use the computer's friendly name rather than Syncthing's default (the raw
		// hostname), and follow it when the computer is renamed, unless a name was
		// chosen with -name.
		if me, _, ok := c.Device(myID); ok && o.DeviceName != "" && (o.ForceName || !custom) && me.Name != o.DeviceName {
			me.Name = o.DeviceName
			c.SetDevice(me)
		}
	})
	if err != nil {
		return err
	}
	w.Wait()
	return nil
}

// Name is this device's name as other devices see it.
func (e *Engine) Name() string {
	if d, ok := e.cfg.Device(e.ID); ok {
		return d.Name
	}
	return ""
}

// AddPeer trusts a device and shares the folder with it. Syncthing then connects
// on its own, over the local network or through relays. Addresses already known
// (say, from the device answering on the local network) are tried first, so the
// connection starts at once instead of waiting for discovery.
func (e *Engine) AddPeer(id protocol.DeviceID, name string, addrs ...string) error {
	w, err := e.cfg.Modify(func(c *config.Configuration) {
		d, _, ok := c.Device(id)
		if !ok {
			d = c.Defaults.Device.Copy()
			d.DeviceID = id
			d.Addresses = append(addrs, "dynamic")
		}
		if name != "" {
			d.Name = name
		}
		c.SetDevice(d)
		if f, _, ok := c.Folder(e.FolderID); ok && !f.SharedWith(id) {
			f.Devices = append(f.Devices, config.FolderDeviceConfiguration{DeviceID: id})
			c.SetFolder(f)
		}
	})
	if err != nil {
		return err
	}
	w.Wait()
	return nil
}

// SyncPort is the port Syncthing accepts connections on (TCP and QUIC share it).
func (e *Engine) SyncPort() int {
	for _, a := range e.cfg.Options().ListenAddresses() {
		if u, err := url.Parse(a); err == nil && (u.Scheme == "tcp" || u.Scheme == "quic") {
			if p, err := strconv.Atoi(u.Port()); err == nil && p > 0 {
				return p
			}
		}
	}
	return 0
}

// RemovePeer stops sharing with a device and forgets it.
func (e *Engine) RemovePeer(id protocol.DeviceID) error {
	w, err := e.cfg.Modify(func(c *config.Configuration) {
		if f, _, ok := c.Folder(e.FolderID); ok {
			kept := f.Devices[:0]
			for _, d := range f.Devices {
				if d.DeviceID != id {
					kept = append(kept, d)
				}
			}
			f.Devices = kept
			c.SetFolder(f)
		}
		if _, i, ok := c.Device(id); ok {
			c.Devices = append(c.Devices[:i], c.Devices[i+1:]...)
		}
	})
	if err != nil {
		return err
	}
	w.Wait()
	return nil
}

// Config exposes Syncthing's configuration for pairing and folder sharing.
func (e *Engine) Config() config.Wrapper { return e.cfg }

// Internals gives direct access to sync state (unstable upstream API; keep usage small).
func (e *Engine) Internals() *syncthing.Internals { return e.app.Internals }

// Stop shuts Syncthing down cleanly.
func (e *Engine) Stop() {
	e.app.Stop(svcutil.ExitSuccess)
	e.app.Wait()
	e.cancel()
}

func randomKey() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
