// SameDesk: a shared clipboard, files and notes across your computers,
// synced by an embedded Syncthing. One program, nothing else to install.
package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/engine"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/hub"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/tray"
)

func main() {
	home, _ := os.UserHomeDir()
	confDir, _ := os.UserConfigDir()

	dataDir := flag.String("data", filepath.Join(confDir, "SameDesk"), "where SameDesk keeps its settings and sync database")
	folder := flag.String("folder", filepath.Join(home, "SameDesk"), "the shared folder")
	port := flag.Int("port", 8765, "dashboard port")
	openUI := flag.Bool("open", true, "open the dashboard in a browser on start")
	name := flag.String("name", "", "how this device appears to others (default: the computer's name)")
	useTray := flag.Bool("tray", true, "show SameDesk in the menu bar / notification area (off: run in the background only)")
	flag.Parse()
	*dataDir, _ = filepath.Abs(*dataDir)
	*folder, _ = filepath.Abs(*folder)
	withTray := *useTray && tray.Available()

	// Bind the dashboard port first: it doubles as the "already running" check.
	srv, err := hub.Listen(*port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SameDesk is already running (port %d is in use). Open http://localhost:%d\n", *port, *port)
		if *openUI {
			hub.OpenBrowser(fmt.Sprintf("http://localhost:%d", *port))
		}
		os.Exit(0)
	}
	logTo(*dataDir) // only now: a second copy must not wipe the running one's log

	eng, err := engine.Start(engine.Options{
		HomeDir:    filepath.Join(*dataDir, "syncthing"),
		FolderPath: *folder,
		FolderID:   "samedesk",
		FolderName: "SameDesk",
		DeviceName: firstNonEmpty(*name, hub.DisplayName()),
		ForceName:  *name != "",
	})
	if err != nil {
		slog.Error("Could not start sync engine", "error", err)
		os.Exit(1)
	}
	slog.Info("Sync engine running", "device", eng.ID.Short().String(), "folder", *folder)

	h, err := hub.New(hub.Config{Root: *folder, DataDir: *dataDir, Port: *port, Engine: eng})
	if err != nil {
		slog.Error("Could not start dashboard", "error", err)
		eng.Stop()
		os.Exit(1)
	}
	go func() {
		if err := srv.Serve(h); err != nil {
			slog.Error("Dashboard stopped", "error", err)
		}
	}()
	url := fmt.Sprintf("http://localhost:%d", *port)
	slog.Info("SameDesk is ready", "url", url)
	if *openUI {
		hub.OpenBrowser(url)
	}

	shutdown := func() {
		slog.Info("Shutting down")
		_ = srv.Close()
		eng.Stop()
	}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	if !withTray {
		<-stop
		shutdown()
		return
	}
	go func() { <-stop; tray.Quit() }()
	tray.Run(tray.Options{Hub: h, LoginArgs: loginArgs(), OnExit: shutdown})
}

// loginArgs are the flags to start with at login: whatever was set now, minus
// opening the browser, which nobody wants at every login.
func loginArgs() []string {
	args := []string{"-open=false"}
	flag.Visit(func(f *flag.Flag) {
		if f.Name != "open" {
			args = append(args, "-"+f.Name+"="+f.Value.String())
		}
	})
	return args
}

// logTo keeps a log next to the settings. A tray app has no terminal to print to
// (on Windows it has no console at all), so this is where errors end up.
func logTo(dir string) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "samedesk.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return
	}
	// The file first: MultiWriter stops at a failed write, and a GUI program's stderr may not exist.
	slog.SetDefault(slog.New(slog.NewTextHandler(io.MultiWriter(f, os.Stderr), nil)))
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
