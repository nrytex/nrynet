package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

const maxCrashLogBytes = 4 * 1024 * 1024

func configureCrashOutput() {
	directory, err := os.UserConfigDir()
	if err != nil {
		return
	}
	crashDirectory := filepath.Join(directory, "Nrynet")
	if err := os.MkdirAll(crashDirectory, 0o750); err != nil {
		return
	}
	path := filepath.Join(crashDirectory, "crash.log")
	if info, err := os.Stat(path); err == nil && info.Size() > maxCrashLogBytes {
		_ = os.Truncate(path, 0)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return
	}
	if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err != nil {
		_ = file.Close()
		return
	}
	_ = file.Close()
}
