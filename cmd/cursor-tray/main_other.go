//go:build !windows && !darwin

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "cursor-tray: supported on Windows (system tray) and macOS (RunCat Neo metrics)")
	os.Exit(1)
}
