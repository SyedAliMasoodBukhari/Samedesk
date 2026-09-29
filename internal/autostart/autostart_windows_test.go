package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// keep saves a registry value and puts it back afterwards, so running the tests
// on a real PC doesn't change its startup settings.
func keep(t *testing.T, path string, binary bool) {
	var str string
	var bin []byte
	had := false
	if k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE); err == nil {
		if binary {
			bin, _, err = k.GetBinaryValue(value)
		} else {
			str, _, err = k.GetStringValue(value)
		}
		had = err == nil
		k.Close()
	}
	t.Cleanup(func() {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.SET_VALUE)
		if err != nil {
			return
		}
		defer k.Close()
		switch {
		case !had:
			_ = k.DeleteValue(value)
		case binary:
			_ = k.SetBinaryValue(value, bin)
		default:
			_ = k.SetStringValue(value, str)
		}
	})
}

func TestStartAtLogin(t *testing.T) {
	keep(t, runKey, false)
	keep(t, approvedKey, true)

	if err := Disable(); err != nil {
		t.Fatal(err)
	}
	if Enabled() {
		t.Fatal("enabled after Disable")
	}
	if err := Enable("-open=false"); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Fatal("not enabled after Enable")
	}

	// Switched off in Task Manager's Startup apps: the Run entry stays, but Windows won't start it.
	k, _, err := registry.CreateKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	_ = k.SetBinaryValue(value, []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	k.Close()
	if Enabled() {
		t.Error("reported enabled while Startup apps has it switched off")
	}
	if err := Enable("-open=false"); err != nil {
		t.Fatal(err)
	}
	if !Enabled() {
		t.Error("Enable didn't undo the Startup apps switch")
	}
	if err := Disable(); err != nil || Enabled() {
		t.Errorf("Disable: err=%v, still enabled=%v", err, Enabled())
	}
}
