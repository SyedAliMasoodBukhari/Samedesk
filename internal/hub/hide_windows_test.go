package hub

import (
	"testing"

	"golang.org/x/sys/windows"
)

// isHidden is what Explorer sees: the hidden attribute, whatever the name.
func isHidden(t *testing.T, p string) bool {
	name, _ := windows.UTF16PtrFromString(p)
	attrs, err := windows.GetFileAttributes(name)
	if err != nil {
		t.Fatal(err)
	}
	return attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0
}
