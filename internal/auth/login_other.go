//go:build !windows

package auth

import "errors"

func loginFlow(dataPath string) (Result, error) {
	return Result{}, errors.New("auth: windows only")
}
