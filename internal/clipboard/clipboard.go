// Package clipboard reads and writes the system clipboard using each OS's own
// tools (pbcopy, PowerShell, wl-clipboard or xclip), so it needs no cgo.
package clipboard

import "errors"

// ErrUnavailable means no clipboard tool was found (e.g. a Linux box without
// wl-clipboard, xclip or xsel).
var ErrUnavailable = errors.New("no clipboard tool available")
