package clipboard

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func powershell(script string) *exec.Cmd {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-STA", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

func ReadText() (string, error) {
	out, err := powershell(`[Console]::OutputEncoding = [Text.Encoding]::UTF8; Get-Clipboard -Raw`).Output()
	return strings.TrimSuffix(string(out), "\r\n"), err
}

func WriteText(s string) error {
	cmd := powershell(`[Console]::InputEncoding = [Text.Encoding]::UTF8; Set-Clipboard -Value ([Console]::In.ReadToEnd())`)
	cmd.Stdin = strings.NewReader(s)
	return cmd.Run()
}

// WriteImage puts an image file on the clipboard.
func WriteImage(path string) error {
	cmd := powershell(`Add-Type -AssemblyName System.Windows.Forms, System.Drawing; ` +
		`$img = [System.Drawing.Image]::FromFile($env:SAMEDESK_IMG); [System.Windows.Forms.Clipboard]::SetImage($img); $img.Dispose()`)
	cmd.Env = append(os.Environ(), "SAMEDESK_IMG="+path)
	return cmd.Run()
}
