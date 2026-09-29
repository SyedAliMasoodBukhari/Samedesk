package autostart

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	value  = "SameDesk"
	// Task Manager's Startup apps switch. A value whose first byte is odd means
	// "disabled"; no value means enabled.
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// installed: where the installer puts it, %LOCALAPPDATA%\Programs\SameDesk.
func installed() bool {
	exe, err := executable()
	want := filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "SameDesk")
	return err == nil && os.Getenv("LOCALAPPDATA") != "" && strings.EqualFold(filepath.Dir(exe), want)
}

// Enabled means Windows will really start it: the Run entry is there and
// Startup apps hasn't switched it off.
func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	if _, _, err = k.GetStringValue(value); err != nil {
		return false
	}
	return !switchedOff()
}

func switchedOff() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(value)
	return err == nil && len(b) > 0 && b[0]&1 == 1
}

func Enable(args ...string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	cmd := []string{`"` + exe + `"`}
	for _, a := range args {
		if strings.ContainsAny(a, " \t") {
			a = `"` + a + `"`
		}
		cmd = append(cmd, a)
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue(value, strings.Join(cmd, " ")); err != nil {
		return err
	}
	// Turning it on here also undoes "Disabled" in Task Manager's Startup apps.
	if a, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE); err == nil {
		_ = a.DeleteValue(value)
		a.Close()
	}
	return nil
}

func Disable() error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	if err := k.DeleteValue(value); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}
