//go:build darwin

package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	launchAgentName       = "io.nrynet.desktop.plist"
	legacyLaunchAgentName = "io.natlink.desktop.plist"
)

func SetAutoStart(enabled bool) error {
	dir, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "Library", "LaunchAgents", launchAgentName)
	legacyPath := filepath.Join(dir, "Library", "LaunchAgents", legacyLaunchAgentName)
	if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	if !enabled {
		if err := os.Remove(path); os.IsNotExist(err) {
			return nil
		} else {
			return err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(plist(exe)), 0o644)
}

func plist(exe string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
"http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>io.nrynet.desktop</string>
<key>ProgramArguments</key><array><string>%s</string></array>
<key>RunAtLoad</key><true/>
</dict></plist>
`, exe)
}
