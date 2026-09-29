package hub

import "golang.org/x/sys/windows"

// hideNative sets the hidden attribute. Explorer ignores the leading dot that hides
// files on macOS and Linux, so .samedesk, .stfolder and the like need it too.
func hideNative(paths []string) {
	for _, p := range paths {
		name, err := windows.UTF16PtrFromString(p)
		if err != nil {
			continue
		}
		attrs, err := windows.GetFileAttributes(name)
		if err != nil || attrs&windows.FILE_ATTRIBUTE_HIDDEN != 0 {
			continue
		}
		_ = windows.SetFileAttributes(name, attrs|windows.FILE_ATTRIBUTE_HIDDEN)
	}
}
