package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"strings"
)

const label = "io.github.samedesk"

func plistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

// installed: running from inside an app bundle, as the .dmg installs it.
func installed() bool {
	exe, err := executable()
	return err == nil && strings.Contains(exe, ".app/Contents/MacOS/")
}

func Enabled() bool { _, err := os.Stat(plistPath()); return err == nil }

// Enable writes the LaunchAgent. launchd picks it up at the next login; it is
// not loaded now, because SameDesk is already running.
func Enable(args ...string) error {
	exe, err := executable()
	if err != nil {
		return err
	}
	var argv bytes.Buffer
	for _, a := range append([]string{exe}, args...) {
		argv.WriteString("\t\t<string>")
		_ = xml.EscapeText(&argv, []byte(a))
		argv.WriteString("</string>\n")
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
` + argv.String() + `	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
</dict>
</plist>
`
	if err := os.MkdirAll(filepath.Dir(plistPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(plistPath(), []byte(plist), 0o644)
}

func Disable() error {
	if err := os.Remove(plistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
