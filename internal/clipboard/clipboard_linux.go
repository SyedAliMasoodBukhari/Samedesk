package clipboard

import (
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func have(tool string) bool { _, err := exec.LookPath(tool); return err == nil }

func wayland() bool { return os.Getenv("WAYLAND_DISPLAY") != "" && have("wl-copy") }

func ReadText() (string, error) {
	switch {
	case wayland():
		out, err := exec.Command("wl-paste", "--no-newline").Output()
		return string(out), err
	case have("xclip"):
		out, err := exec.Command("xclip", "-selection", "clipboard", "-o").Output()
		return string(out), err
	case have("xsel"):
		out, err := exec.Command("xsel", "--clipboard", "--output").Output()
		return string(out), err
	}
	return "", ErrUnavailable
}

func WriteText(s string) error {
	var cmd *exec.Cmd
	switch {
	case wayland():
		cmd = exec.Command("wl-copy")
	case have("xclip"):
		cmd = exec.Command("xclip", "-selection", "clipboard")
	case have("xsel"):
		cmd = exec.Command("xsel", "--clipboard", "--input")
	default:
		return ErrUnavailable
	}
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// WriteImage puts an image file on the clipboard.
func WriteImage(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	typ := mime.TypeByExtension(filepath.Ext(path))
	if typ == "" {
		typ = "image/png"
	}
	var cmd *exec.Cmd
	switch {
	case wayland():
		cmd = exec.Command("wl-copy", "--type", typ)
	case have("xclip"):
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", typ)
	default:
		return ErrUnavailable
	}
	cmd.Stdin = f
	return cmd.Run()
}
