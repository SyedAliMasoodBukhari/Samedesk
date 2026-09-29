// Package autostart starts SameDesk when the user logs in: a LaunchAgent on macOS,
// the Run key on Windows and an XDG autostart entry on Linux. Nothing needs admin rights.
package autostart

import (
	"os"
	"path/filepath"
)

// executable is the running program, with symlinks resolved so a Homebrew-style
// link or a moved download still points at the real binary.
func executable() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p, nil
}
