//go:build !windows

package settings

import "errors"

func openWindow(d Deps) error {
	return errors.New("settings: windows only")
}
