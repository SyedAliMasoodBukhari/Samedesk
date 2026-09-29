package clipboard

import (
	"os"
	"os/exec"
	"strings"
)

// pbcopy and pbpaste pick the text encoding from the locale; pin it to UTF-8.
func utf8Env() []string { return append(os.Environ(), "LANG=en_US.UTF-8", "LC_ALL=en_US.UTF-8") }

func ReadText() (string, error) {
	cmd := exec.Command("pbpaste")
	cmd.Env = utf8Env()
	out, err := cmd.Output()
	return string(out), err
}

func WriteText(s string) error {
	cmd := exec.Command("pbcopy")
	cmd.Env = utf8Env()
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// WriteImage puts an image file on the clipboard (PNG, JPEG, GIF or TIFF).
func WriteImage(path string) error {
	script := `on run argv
	set the clipboard to (read (POSIX file (item 1 of argv)) as «class PNGf»)
end run`
	if !strings.HasSuffix(strings.ToLower(path), ".png") {
		script = `on run argv
	set the clipboard to (read (POSIX file (item 1 of argv)) as picture)
end run`
	}
	return exec.Command("osascript", "-e", script, path).Run()
}
