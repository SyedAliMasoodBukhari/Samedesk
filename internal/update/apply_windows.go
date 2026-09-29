package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
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
	cmd := exec.Command(setup, "/S", "/RELAUNCH")
	const detached, newGroup = 0x00000008, 0x00000200 // DETACHED_PROCESS, CREATE_NEW_PROCESS_GROUP
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detached | newGroup, HideWindow: true}
	return cmd.Start()
}

func cleanupOld() {}
