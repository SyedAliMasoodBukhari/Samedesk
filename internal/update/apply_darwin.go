package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func assetName(v string) string { return "SameDesk-" + v + ".dmg" }

// bundle is the SameDesk.app this copy runs from, if any.
func bundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(exe); err == nil {
		exe = r
	}
	i := strings.Index(exe, ".app/Contents/MacOS/")
	if i < 0 {
		return ""
	}
	return exe[:i+len(".app")]
}

// selfUpdatable: an app bundle in a folder we may write to (Applications is,
// for an administrator).
func selfUpdatable() bool {
	b := bundle()
	return b != "" && unix.Access(filepath.Dir(b), unix.W_OK) == nil
}

// apply swaps the app bundle for the one in the .dmg and starts it. The new copy
// waits for this one to quit (-restarted) before taking over.
func apply(dmg string, args []string) error {
	app := bundle()
	if app == "" {
		return fmt.Errorf("not running from an app bundle")
	}
	return applyTo(app, dmg, args)
}

func applyTo(app, dmg string, args []string) error {
	mnt := filepath.Join(filepath.Dir(dmg), "mnt")
	_ = os.MkdirAll(mnt, 0o700)
	if out, err := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-noautoopen", "-mountpoint", mnt, dmg).CombinedOutput(); err != nil {
		return fmt.Errorf("mount: %v: %s", err, out)
	}
	defer exec.Command("hdiutil", "detach", "-quiet", "-force", mnt).Run()

	staged, old := app+".new", app+".old"
	_ = os.RemoveAll(staged)
	_ = os.RemoveAll(old)
	if out, err := exec.Command("ditto", filepath.Join(mnt, "SameDesk.app"), staged).CombinedOutput(); err != nil {
		return fmt.Errorf("copy: %v: %s", err, out)
	}
	if out, err := exec.Command("codesign", "--verify", "--strict", staged).CombinedOutput(); err != nil {
		_ = os.RemoveAll(staged)
		return fmt.Errorf("new app's signature: %v: %s", err, out)
	}
	// Two renames, so SameDesk.app is never missing for more than an instant.
	if err := os.Rename(app, old); err != nil {
		_ = os.RemoveAll(staged)
		return err
	}
	if err := os.Rename(staged, app); err != nil {
		_ = os.Rename(old, app)
		return err
	}
	_ = os.RemoveAll(old)
	return exec.Command("open", append([]string{"-n", "-a", app, "--args"}, args...)...).Run()
}

// cleanupOld removes a half-finished swap.
func cleanupOld() {
	if b := bundle(); b != "" {
		_ = os.RemoveAll(b + ".new")
		_ = os.RemoveAll(b + ".old")
	}
}
