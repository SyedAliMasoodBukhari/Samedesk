package hub

import (
	"io/fs"
	"os"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Everything here calls Windows directly rather than starting helper programs
// (PowerShell, rundll32): simpler, and nothing that looks like running commands.

// hereTime is when the file was created on this PC (Syncthing sets the
// sender's mtime, so this is when it actually arrived).
func hereTime(info fs.FileInfo) time.Time {
	if d, ok := info.Sys().(*syscall.Win32FileAttributeData); ok {
		return time.Unix(0, d.CreationTime.Nanoseconds())
	}
	return time.Time{}
}

var shFileOperation = windows.NewLazySystemDLL("shell32.dll").NewProc("SHFileOperationW")

// shFileOp is SHFILEOPSTRUCTW (64-bit layout).
type shFileOp struct {
	hwnd          uintptr
	fn            uint32
	from, to      *uint16
	flags         uint16
	aborted       int32
	nameMappings  uintptr
	progressTitle *uint16
}

// systemTrash moves a file or folder to the Recycle Bin.
func systemTrash(p string) error {
	const (
		foDelete         = 0x3
		fofSilent        = 0x4
		fofNoConfirm     = 0x10
		fofAllowUndo     = 0x40
		fofNoErrorUI     = 0x400
		fofNoConfirmMkdr = 0x200
	)
	// pFrom is a list of paths, each ended by a NUL, the list by another.
	from, err := windows.UTF16FromString(p)
	if err != nil {
		return err
	}
	from = append(from, 0)
	op := shFileOp{fn: foDelete, from: &from[0], flags: fofAllowUndo | fofNoConfirm | fofSilent | fofNoErrorUI | fofNoConfirmMkdr}
	if r, _, _ := shFileOperation.Call(uintptr(unsafe.Pointer(&op))); r != 0 {
		return syscall.Errno(r)
	}
	if op.aborted != 0 {
		return os.ErrPermission
	}
	return nil
}

func shellOpen(file, args string) {
	f, err := windows.UTF16PtrFromString(file)
	if err != nil {
		return
	}
	var a *uint16
	if args != "" {
		a, _ = windows.UTF16PtrFromString(args)
	}
	verb, _ := windows.UTF16PtrFromString("open")
	_ = windows.ShellExecute(0, verb, f, a, nil, windows.SW_SHOWNORMAL)
}

func reveal(p string) { shellOpen("explorer.exe", `/select,"`+p+`"`) }

// OpenBrowser opens the dashboard (or any link or folder) the way a double-click would.
func OpenBrowser(url string) { shellOpen(url, "") }

// DisplayName is the PC's name.
func DisplayName() string {
	h, _ := os.Hostname()
	return h
}
