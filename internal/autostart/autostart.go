// Package autostart starts SameDesk when the user logs in: a LaunchAgent on macOS,
// the Run key on Windows and an XDG autostart entry on Linux. Nothing needs admin rights.
package autostart

import (
	"os"
	"path/filepath"
)

// Installed reports whether this is a copy put in place by an installer or
// package (as opposed to a build run from a checkout). Only installed copies
// turn Start at Login on by themselves.
func Installed() bool { return installed() }

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
