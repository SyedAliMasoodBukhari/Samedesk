package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func assetName(v string) string {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	return "SameDesk-" + v + "-windows-" + arch + "-setup.exe"
}

// selfUpdatable: installed by the SameDesk installer, which can install over itself.
func selfUpdatable() bool {
	exe, err := os.Executable()
	want := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "SameDesk")
	return err == nil && os.Getenv("LOCALAPPDATA") != "" && strings.EqualFold(filepath.Dir(exe), want)
}

// apply runs the new installer silently. It waits for this copy to quit,
// replaces it and starts the new version (/RELAUNCH).
func apply(setup string, args []string) error {
	// A plain start: the installer outlives this copy on its own.
	return exec.Command(setup, "/S", "/RELAUNCH").Start()
}

func cleanupOld() {}
