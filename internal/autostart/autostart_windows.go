package autostart

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	value  = "SameDesk"
)

func Enabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(value)
	return err == nil
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
	return k.SetStringValue(value, strings.Join(cmd, " "))
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
