//go:build windows

// Package autostart registers/removes the app from the current user's
// startup programs (HKCU Run key).
package autostart

import (
	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// SetEnabled adds or removes the executable from the HKCU Run key.
func SetEnabled(appName, exePath string, enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()

	if enabled {
		return k.SetStringValue(appName, `"`+exePath+`"`)
	}
	// Delete only if the current value points at this executable, to avoid
	// clobbering an unrelated install under the same name.
	if v, _, err := k.GetStringValue(appName); err == nil && v == `"`+exePath+`"` {
		return k.DeleteValue(appName)
	}
	return nil
}

// Enabled reports whether the Run key currently starts this executable.
func Enabled(appName, exePath string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetStringValue(appName)
	return err == nil && v == `"`+exePath+`"`
}
