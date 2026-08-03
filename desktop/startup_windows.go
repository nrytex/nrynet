//go:build windows

package main

import (
	"errors"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	autoStartName       = "Nrynet"
	legacyAutoStartName = "NAT-Link"
)

func SetAutoStart(enabled bool) error {
	key, _, err := registry.CreateKey(
		registry.CURRENT_USER,
		`Software\Microsoft\Windows\CurrentVersion\Run`,
		registry.SET_VALUE,
	)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(legacyAutoStartName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	if !enabled {
		if err := key.DeleteValue(autoStartName); errors.Is(err, registry.ErrNotExist) {
			return nil
		} else {
			return err
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return key.SetStringValue(autoStartName, `"`+exe+`"`)
}
