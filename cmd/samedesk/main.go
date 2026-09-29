// SameDesk: a shared clipboard, files and notes across your computers,
// synced by an embedded Syncthing. One program, nothing else to install.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/SyedAliMasoodBukhari/samedesk/internal/engine"
	"github.com/SyedAliMasoodBukhari/samedesk/internal/hub"
)

func main() {
	home, _ := os.UserHomeDir()
	confDir, _ := os.UserConfigDir()

	dataDir := flag.String("data", filepath.Join(confDir, "SameDesk"), "where SameDesk keeps its settings and sync database")
	folder := flag.String("folder", filepath.Join(home, "SameDesk"), "the shared folder")
	port := flag.Int("port", 8765, "dashboard port")
	openUI := flag.Bool("open", true, "open the dashboard in a browser on start")
	name := flag.String("name", "", "how this device appears to others (default: the computer's name)")
	flag.Parse()

	// Bind the dashboard port first: it doubles as the "already running" check.
	srv, err := hub.Listen(*port)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SameDesk is already running (port %d is in use). Open http://localhost:%d\n", *port, *port)
		if *openUI {
			hub.OpenBrowser(fmt.Sprintf("http://localhost:%d", *port))
		}
		os.Exit(0)
	}

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

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	slog.Info("Shutting down")
	_ = srv.Close()
	eng.Stop()
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}
