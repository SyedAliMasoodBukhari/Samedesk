package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// assetName: only an AppImage can replace itself; packages go through apt or dnf.
func assetName(v string) string {
	if os.Getenv("APPIMAGE") == "" {
		return ""
	}
	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "aarch64"
	}
	return "SameDesk-" + v + "-" + arch + ".AppImage"
}

func selfUpdatable() bool {
	img := os.Getenv("APPIMAGE")
	return img != "" && unix.Access(filepath.Dir(img), unix.W_OK) == nil
}

// apply puts the new AppImage where the old one was and starts it.
func apply(file string, args []string) error {
	img := os.Getenv("APPIMAGE")
	if img == "" {
		return fmt.Errorf("not an AppImage")
	}
	staged := filepath.Join(filepath.Dir(img), "."+filepath.Base(img)+".new")
	in, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if err := os.WriteFile(staged, in, 0o755); err != nil {
		return err
	}
	if err := os.Rename(staged, img); err != nil {
		_ = os.Remove(staged)
		return err
	}
	cmd := exec.Command(img, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}

func cleanupOld() {
	if img := os.Getenv("APPIMAGE"); img != "" {
		_ = os.Remove(filepath.Join(filepath.Dir(img), "."+filepath.Base(img)+".new"))
	}
}
